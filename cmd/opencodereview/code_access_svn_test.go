// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSVNComparisonSideCodeToolsRealRepository(t *testing.T) {
	for _, name := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s unavailable", name)
		}
	}
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	wc := filepath.Join(tmp, "wc")
	run := func(dir, name string, args ...string) {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v: %s", name, args, err, out)
		}
	}
	run(tmp, "svnadmin", "create", repo)
	run(tmp, "svn", "checkout", "file://"+filepath.ToSlash(repo), wc)
	file := filepath.Join(wc, "space \u4e2d\u6587.c")
	if err := os.WriteFile(file, []byte("OLD_SIDE_MARKER\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(wc, "svn", "add", "space \u4e2d\u6587.c")
	run(wc, "svn", "commit", "-m", "first")
	if err := os.WriteFile(file, []byte("NEW_SIDE_MARKER\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(wc, "svn", "commit", "-m", "second")
	for _, opts := range []reviewOptions{{commit: "2"}, {from: "1", to: "2"}} {
		setup, err := prepareReviewCodeTools(context.Background(), wc, opts, nil, nil, true)
		if err != nil {
			t.Fatal(err)
		}
		if setup.access == nil || !setup.reader.SVN {
			t.Fatalf("SVN comparison was disabled: %+v", setup)
		}
		registry := buildToolRegistryWithCodeAccess(nil, setup.reader, setup.access)
		for _, tc := range []struct{ side, marker string }{{"old", "OLD_SIDE_MARKER"}, {"new", "NEW_SIDE_MARKER"}} {
			for _, toolName := range []string{"file_read", "code_search", "file_find"} {
				p, ok := registry.Get(toolName)
				if !ok {
					t.Fatal(toolName)
				}
				args := map[string]any{"side": tc.side, "file_path": "space \u4e2d\u6587.c", "search_text": tc.marker, "query_name": "space \u4e2d\u6587.c"}
				got, err := p.Execute(context.Background(), args)
				want := tc.marker
				if toolName == "file_find" {
					want = "space \u4e2d\u6587.c"
				}
				if err != nil || !strings.Contains(got, want) {
					t.Errorf("%+v %s %s: %q %v", opts, tc.side, toolName, got, err)
				}
			}
		}
	}
}
