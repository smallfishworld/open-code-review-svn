// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

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
	for _, path := range p.unversionedFiles(items) {
		content, readErr := readWorkspaceFileForDiff(p.repoDir, path)
		if readErr == nil {
			combined += newFileUnifiedDiff(path, content) + "\n\n"
		}
	}
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

func newFileUnifiedDiff(path string, content []byte) string {
	if bytes.IndexByte(content, 0) >= 0 || !utf8.Valid(content) {
		return fmt.Sprintf("diff --git a/%s b/%s\nnew file mode 100644\nBinary files a/%s and b/%s differ\n", path, path, path, path)
	}
	lineCount := bytes.Count(content, []byte{'\n'})
	if len(content) > 0 && content[len(content)-1] != '\n' {
		lineCount++
	}
	var out strings.Builder
	fmt.Fprintf(&out, "diff --git a/%s b/%s\nnew file mode 100644\n--- /dev/null\n+++ b/%s\n@@ -0,0 +1,%d @@\n", path, path, path, lineCount)
	lines := bytes.Split(content, []byte{'\n'})
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		out.WriteByte('+')
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.String()
}

func (p *Provider) unversionedFiles(statuses map[string]string) []string {
	seen := make(map[string]struct{})
	for path, item := range statuses {
		if item != "unversioned" {
			continue
		}
		fullPath := filepath.Join(p.repoDir, filepath.FromSlash(path))
		info, err := os.Lstat(fullPath)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			seen[path] = struct{}{}
			continue
		}
		if !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(fullPath, func(candidate string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if candidate != fullPath && (entry.Name() == ".svn" || entry.Name() == ".git") {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Type().IsRegular() {
				rel, relErr := filepath.Rel(p.repoDir, candidate)
				if relErr == nil {
					seen[filepath.ToSlash(rel)] = struct{}{}
				}
			}
			return nil
		})
	}
	files := make([]string, 0, len(seen))
	for path := range seen {
		files = append(files, path)
	}
	sort.Strings(files)
	return files
}

func (p *Provider) svnRemoteIdentity(ctx context.Context) string {
	out, err := p.runSVN(ctx, "info", "--show-item", "repos-root-url")
	if err != nil {
		return ""
	}
	return canonicalRemote(strings.TrimSpace(string(out)))
}
