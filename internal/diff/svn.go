// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/alibaba/open-code-review/internal/model"
)

type svnStatus struct {
	Entries []struct {
		Path string `xml:"path,attr"`
		WC   struct {
			Item string `xml:"item,attr"`
		} `xml:"wc-status"`
	} `xml:"target>entry"`
}

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

func (p *Provider) getSVNWorkspaceDiff(ctx context.Context) ([]model.Diff, error) {
	statusOut, err := p.runSVN(ctx, "status", "--xml")
	if err != nil {
		return nil, fmt.Errorf("svn status: %w", err)
	}
	var status svnStatus
	if err := xml.Unmarshal(statusOut, &status); err != nil {
		return nil, fmt.Errorf("parse svn status: %w", err)
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
		return nil, fmt.Errorf("svn diff: %w", err)
	}
	combined := convertSVNDiff(string(diffOut), items)
	diffs, err := ParseDiffText(ctx, combined, p.repoDir, "", p.runner)
	if err != nil {
		return nil, err
	}
	return p.filterDiffs(diffs), nil
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
		if strings.HasPrefix(line, "--- ") && statuses[svnHeaderPath(line[4:])] == "added" {
			out.WriteString("new file mode 100644\n--- /dev/null\n")
			continue
		}
		if strings.HasPrefix(line, "+++ ") && statuses[svnHeaderPath(line[4:])] == "deleted" {
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
