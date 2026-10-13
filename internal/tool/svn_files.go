// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"encoding/xml"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
)

const svnSearchTimeout = 30 * time.Second

// svnFiles returns versioned files only. Workspace mode sees scheduled additions
// and local edits; history mode enumerates the repository tree at the target
// revision rather than the current working tree. No Git command is invoked.
func svnFiles(parent context.Context, fr *FileReader) ([]string, error) {
	ctx, cancel := context.WithTimeout(parent, svnSearchTimeout)
	defer cancel()

	type entry struct {
		Path string `xml:"path,attr"`
		Kind string `xml:"kind,attr"`
		Name string `xml:"name"`
		WC struct {
			Item string `xml:"item,attr"`
		} `xml:"wc-status"`
	}
	type statusXML struct {
		Entries []entry `xml:"target>entry"`
	}
	type listXML struct {
		Entries []entry `xml:"list>entry"`
	}

	var args []string
	if fr.Mode == ModeWorkspace {
		args = []string{"status", "--xml", "--verbose", "--ignore-externals", "--depth", "infinity"}
	} else {
		if fr.Ref == "" {
			return nil, fmt.Errorf("SVN history search requires a revision")
		}
		// The peg revision is essential if the target path was subsequently
		// deleted or replaced. svn list accepts a WC path and resolves its URL.
		args = []string{"list", "--xml", "--recursive", "-r", fr.Ref, ".@" + fr.Ref}
	}

	cmd := exec.CommandContext(ctx, "svn", args...)
	cmd.Dir = fr.RepoDir
	data, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("SVN file listing timed out; narrow file_patterns: %w", ctx.Err())
		}
		return nil, fmt.Errorf("svn %s: %w", strings.Join(args, " "), err)
	}

	files := make([]string, 0)
	seen := make(map[string]bool)
	add := func(name string) {
		name = filepath.ToSlash(name)
		if filepath.IsAbs(name) {
			rel, err := filepath.Rel(fr.RepoDir, filepath.FromSlash(name))
			if err != nil { return }
			name = filepath.ToSlash(rel)
		}
		name = strings.TrimPrefix(name, "./")
		if name == "" || name == "." || name == ".." || strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") {
			return
		}
		if !seen[name] && !shouldSkipFile(name) {
			seen[name] = true
			files = append(files, name)
		}
	}
	if fr.Mode == ModeWorkspace {
		var status statusXML
		if err := xml.Unmarshal(data, &status); err != nil {
			return nil, fmt.Errorf("parse SVN status: %w", err)
		}
		for _, e := range status.Entries {
			switch e.WC.Item {
			case "normal", "modified", "added", "replaced", "conflicted", "merged":
				if e.Kind != "dir" && e.Kind != "directory" && filepath.Ext(e.Path) != "" {
					add(e.Path)
				} else if e.Kind != "dir" && e.Kind != "directory" {
					add(e.Path)
				}
			}
		}
	} else {
		var listing listXML
		if err := xml.Unmarshal(data, &listing); err != nil {
			return nil, fmt.Errorf("parse SVN revision listing: %w", err)
		}
		for _, e := range listing.Entries {
			if e.Kind == "file" {
				add(e.Name)
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// matchSVNFilePattern implements the directory and glob filtering typically
// passed to code_search, without feeding untrusted patterns to shell commands.
func matchSVNFilePattern(file string, patterns []string) bool {
	if len(patterns) == 0 { return true }
	for _, raw := range patterns {
		p := strings.TrimPrefix(strings.ReplaceAll(raw, "\\", "/"), "./")
		if p == "" { continue }
		if strings.HasSuffix(p, "/") && strings.HasPrefix(file, p) { return true }
		if file == p || strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/") { return true }
		if ok, err := doublestar.Match(p, file); err == nil && ok { return true }
		if !strings.Contains(p, "/") {
			if ok, err := doublestar.Match(p, filepath.Base(file)); err == nil && ok { return true }
		}
	}
	return false
}
