// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alibaba/open-code-review/internal/gitcmd"

)

type svnStatus struct {
	Entries []struct {
		Path string `xml:"path,attr"`
		WC   struct {
			Item string `xml:"item,attr"`
		} `xml:"wc-status"`
	} `xml:"target>entry"`
}

// IsSVNWorkingCopy reports a Subversion checkout rather than a Git worktree.
func IsSVNWorkingCopy(dir string) bool { return isSVNWorkingCopy(dir) }

func isSVNWorkingCopy(dir string) bool {
	gitCmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	gitCmd.Dir = dir
	if gitCmd.Run() == nil {
		return false
	}
	cmd := exec.Command("svn", "info", "--show-item", "wc-root", dir)
	return cmd.Run() == nil
}

func (p *Provider) runSVN(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "svn", args...)
	cmd.Dir = p.repoDir
	return cmd.Output()
}

func (p *Provider) getSVNWorkspaceDiffSet(ctx context.Context) (DiffSet, error) {
	statusOut, err := p.runSVN(ctx, "status", "--xml")
	if err != nil {
		return DiffSet{}, fmt.Errorf("svn status: %w", err)
	}
	var status svnStatus
	if err := xml.Unmarshal(statusOut, &status); err != nil {
		return DiffSet{}, fmt.Errorf("parse svn status: %w", err)
	}
	items := make(map[string]string, len(status.Entries))
	for _, entry := range status.Entries {
		path := filepath.ToSlash(entry.Path)
		if filepath.IsAbs(entry.Path) {
			if rel, relErr := filepath.Rel(p.repoDir, entry.Path); relErr == nil {
				path = filepath.ToSlash(rel)
			}
		}
		items[path] = entry.WC.Item
	}

	diffOut, err := p.runSVN(ctx, "diff", "--notice-ancestry", "--depth", "infinity")
	if err != nil {
		return DiffSet{}, fmt.Errorf("svn diff: %w", err)
	}
	combined := convertSVNDiff(string(diffOut), items)
	diffs, err := ParseDiffText(ctx, combined, p.repoDir, "", p.runner)
	if err != nil {
		return DiffSet{}, err
	}
	return p.partitionDiffs(diffs), nil
}

func convertSVNDiff(input string, statuses map[string]string) string {
	var out strings.Builder
	var currentPath string
	for _, line := range strings.Split(input, "\n") {
		if path, ok := strings.CutPrefix(line, "Index: "); ok {
			path = filepath.ToSlash(path)
			currentPath = path
			fmt.Fprintf(&out, "diff --git a/%s b/%s\n", path, path)
			continue
		}
		if strings.HasPrefix(line, "===") {
			continue
		}
		if strings.HasPrefix(line, "Cannot display: file marked as a binary type") {
			fmt.Fprintf(&out, "Binary files a/%s and b/%s differ\n", currentPath, currentPath)
			continue
		}
		if strings.HasPrefix(line, "--- ") && (statuses[svnHeaderPath(line[4:])] == "added" || strings.Contains(line, "(nonexistent)") || strings.Contains(line, "(revision 0)")) {
			out.WriteString("new file mode 100644\n--- /dev/null\n")
			continue
		}
		if strings.HasPrefix(line, "+++ ") && (statuses[svnHeaderPath(line[4:])] == "deleted" || strings.Contains(line, "(nonexistent)")) {
			out.WriteString("deleted file mode 100644\n+++ /dev/null\n")
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

func svnHeaderPath(header string) string {
	if i := strings.IndexByte(header, '\t'); i >= 0 {
		header = header[:i]
	}
	return filepath.ToSlash(header)
}

func (p *Provider) svnRemoteIdentity(ctx context.Context) string {
	out, err := p.runSVN(ctx, "info", "--show-item", "repos-root-url")
	if err != nil {
		return ""
	}
	return canonicalRemote(strings.TrimSpace(string(out)))
}
// NewSVNCommitProvider selects the changes introduced by a single SVN revision.
func NewSVNCommitProvider(repoDir, revision string, runner *gitcmd.Runner) *Provider {
	return &Provider{repoDir: repoDir, mode: ModeCommit, commit: revision, runner: runner, svn: true}
}

// NewSVNRangeProvider compares two SVN repository snapshots directly.
// SVN ranges do not use Git's merge-base behavior.
func NewSVNRangeProvider(repoDir, from, to string, runner *gitcmd.Runner) *Provider {
	return &Provider{repoDir: repoDir, mode: ModeRange, from: from, to: to, runner: runner, svn: true}
}

// getSVNRevisionDiffSet retrieves history from the repository, not the
// possibly dirty working copy. Both boundaries are immutable SVN revisions.
func (p *Provider) getSVNRevisionDiffSet(ctx context.Context) (DiffSet, error) {
	var args []string
	var target string
	switch p.mode {
	case ModeCommit:
		n, err := strconv.ParseUint(p.commit, 10, 64)
		if err != nil || n == 0 {
			return DiffSet{}, fmt.Errorf("SVN revision must be a positive integer: %q", p.commit)
		}
		args = []string{"diff", "--depth", "infinity", "-c", p.commit}
		target = p.commit
	case ModeRange:
		start, e1 := strconv.ParseUint(p.from, 10, 64)
		end, e2 := strconv.ParseUint(p.to, 10, 64)
		if e1 != nil || e2 != nil || start == 0 || start >= end {
			return DiffSet{}, fmt.Errorf("invalid SVN revision range %q:%q", p.from, p.to)
		}
		args = []string{"diff", "--depth", "infinity", "-r", p.from + ":" + p.to}
		target = p.to
	default:
		return DiffSet{}, fmt.Errorf("SVN revision provider requires a commit or range")
	}
	out, err := p.runSVN(ctx, args...)
	if err != nil {
		return DiffSet{}, fmt.Errorf("svn %s: %w", strings.Join(args, " "), err)
	}
	converted := convertSVNDiff(string(out), nil)
	diffs, err := ParseDiffText(ctx, converted, p.repoDir, "svn:"+target, p.runner)
	if err != nil { return DiffSet{}, err }
	return p.partitionDiffs(diffs), nil
}

// ReadSVNRevisionFile reads content as it was in the specified repository
// revision, including files removed or modified in the current working copy.
// The explicit peg revision ensures old paths remain addressable after deletion.
func ReadSVNRevisionFile(ctx context.Context, root, relativePath, revision string) (string, error) {
	n, err := strconv.ParseUint(revision, 10, 64)
	if err != nil || n == 0 {
		return "", fmt.Errorf("invalid SVN revision %q", revision)
	}
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("absolute SVN paths are not allowed")
	}
	clean := filepath.ToSlash(filepath.Clean(relativePath))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("SVN file path %q is outside working copy", relativePath)
	}
	info := exec.CommandContext(ctx, "svn", "info", "--show-item", "url")
	info.Dir = root
	rootURL, err := info.Output()
	if err != nil { return "", fmt.Errorf("svn info url: %w", err) }
	u, err := url.Parse(strings.TrimSpace(string(rootURL)))
	if err != nil || u.Scheme == "" {
		return "", fmt.Errorf("invalid SVN working copy URL")
	}
	u.Path = strings.TrimSuffix(u.Path, "/") + "/" + clean
	u.RawPath = ""
	targetURL := u.String() + "@" + revision
	cmd := exec.CommandContext(ctx, "svn", "cat", "-r", revision, "--", targetURL)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil { return "", fmt.Errorf("svn cat -r %s %q: %w", revision, clean, err) }
	return string(output), nil
}
