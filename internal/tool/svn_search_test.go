// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func svnSearchFixture(t *testing.T) string {
	t.Helper()
	for _, cmd := range []string{"svn", "svnadmin"} {
		if _, err := exec.LookPath(cmd); err != nil {
			t.Skipf("%s is unavailable", cmd)
		}
	}
	root := t.TempDir()
	repository := filepath.Join(root, "repo")
	wc := filepath.Join(root, "wc")
	run := func(dir, cmd string, args ...string) {
		t.Helper()
		c := exec.Command(cmd, args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v: %s", cmd, args, err, out)
		}
	}
	write := func(name, value string) {
		t.Helper()
		p := filepath.Join(wc, name)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(value), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("", "svnadmin", "create", repository)
	run("", "svn", "checkout", "file://"+filepath.ToSlash(repository), wc)
	write("src/hello.c", "const char* msg = \"OLD_DATA_ONLY\";\n")
	write("src/deleted.c", "const char* msg = \"DELETED_AT_R2\";\n")
	write("src/\u4e2d\u6587\u6587\u4ef6.c", "const char* name = \"UTF8_NAME_ONLY\";\n")
	write("space name.c", "const char* space = \"SPACE_NAME_ONLY\";\n")
	run(wc, "svn", "add", "src", "space name.c")
	run(wc, "svn", "commit", "-m", "r1")
	write("src/hello.c", "const char* msg = \"NEW_DATA_ONLY\";\n")
	run(wc, "svn", "delete", "src/deleted.c")
	run(wc, "svn", "commit", "-m", "r2")
	write("src/hello.c", "const char* msg = \"LOCAL_ONLY\";\n")
	write("src/added.c", "const char* msg = \"SCHEDULED_ADD_ONLY\";\n")
	run(wc, "svn", "add", "src/added.c")
	write("untracked.c", "const char* msg = \"UNTRACKED_ONLY\";\n")
	return wc
}

func TestSVNNativeSearchWorkspace(t *testing.T) {
	root := svnSearchFixture(t)
	fr := &FileReader{RepoDir: root, Mode: ModeWorkspace, SVN: true}
	code := NewCodeSearch(fr)
	find := NewFileFind(fr)
	ctx := context.Background()

	for _, tc := range []struct{ text, want string }{
		{"LOCAL_ONLY", "src/hello.c"},
		{"SCHEDULED_ADD_ONLY", "src/added.c"},
		{"UTF8_NAME_ONLY", "src/\u4e2d\u6587\u6587\u4ef6.c"},
		{"SPACE_NAME_ONLY", "space name.c"},
	} {
		got, err := code.Execute(ctx, map[string]any{"search_text": tc.text})
		if err != nil || !strings.Contains(got, tc.want) {
			t.Errorf("SVN workspace search %q: %q, %v", tc.text, got, err)
		}
	}
	if got, err := code.Execute(ctx, map[string]any{"search_text": "UNTRACKED_ONLY"}); err != nil || got != "No matches found" {
		t.Errorf("untracked files should not be searched: %q, %v", got, err)
	}
	if got, err := code.Execute(ctx, map[string]any{
		"search_text": "LOCAL_ONLY", "file_patterns": []any{"src/\u4e2d\u6587*.c"},
	}); err != nil || got != "No matches found" {
		t.Errorf("file_patterns should narrow SVN search: %q, %v", got, err)
	}
	if got, err := code.Execute(ctx, map[string]any{
		"search_text": "local_only", "file_patterns": []any{"src/"},
	}); err != nil || !strings.Contains(got, "src/hello.c") {
		t.Errorf("case insensitive path search: %q, %v", got, err)
	}
	if got, err := code.Execute(ctx, map[string]any{
		"search_text": "LOCAL_[A-Z]+", "use_perl_regexp": true,
	}); err != nil || !strings.Contains(got, "src/hello.c") {
		t.Errorf("regex search: %q, %v", got, err)
	}
	for _, tc := range []struct{ query, want string }{
		{"hello.c", "src/hello.c"},
		{"\u4e2d\u6587\u6587\u4ef6", "src/\u4e2d\u6587\u6587\u4ef6.c"},
		{"added.c", "src/added.c"},
	} {
		got, err := find.Execute(ctx, map[string]any{"query_name": tc.query})
		if err != nil || !strings.Contains(got, tc.want) {
			t.Errorf("SVN file_find %q: %q, %v", tc.query, got, err)
		}
	}
	if got, err := find.Execute(ctx, map[string]any{"query_name": "untracked.c"}); err != nil || !strings.Contains(got, "not found") {
		t.Errorf("untracked file_find: %q, %v", got, err)
	}
}

func TestSVNNativeSearchHistoricalSnapshot(t *testing.T) {
	root := svnSearchFixture(t)
	ctx := context.Background()
	for _, tc := range []struct {
		revision string
		needle   string
		found    bool
	}{
		{"1", "OLD_DATA_ONLY", true},
		{"2", "OLD_DATA_ONLY", false},
		{"2", "NEW_DATA_ONLY", true},
		{"2", "LOCAL_ONLY", false},
		{"1", "DELETED_AT_R2", true},
	} {
		fr := &FileReader{RepoDir: root, SVN: true, Ref: tc.revision, Mode: ModeCommit}
		got, err := NewCodeSearch(fr).Execute(ctx, map[string]any{
			"search_text": tc.needle, "file_patterns": []any{"src/"},
		})
		if err != nil {
			t.Fatalf("revision %s: %v", tc.revision, err)
		}
		found := strings.Contains(got, "File:")
		if found != tc.found {
			t.Errorf("revision %s searching %s: %s", tc.revision, tc.needle, got)
		}
	}
	for _, tc := range []struct {
		revision string
		found    bool
	}{
		{"1", true},
		{"2", false},
	} {
		fr := &FileReader{RepoDir: root, SVN: true, Ref: tc.revision, Mode: ModeRange}
		got, err := NewFileFind(fr).Execute(ctx, map[string]any{"query_name": "deleted.c"})
		if err != nil {
			t.Fatal(err)
		}
		found := strings.Contains(got, "src/deleted.c")
		if found != tc.found {
			t.Errorf("file_find at r%s: %s", tc.revision, got)
		}
	}
}

func TestSVNPatternsAndTraversal(t *testing.T) {
	for _, tc := range []struct {
		name     string
		patterns []string
		ok       bool
	}{
		{"src/main.c", []string{"src/"}, true},
		{"src/main.c", []string{"*.c"}, true},
		{"src/main.c", []string{"src/**/*.c"}, true},
		{"test/main.c", []string{"src/"}, false},
		{"src/\u4e2d\u6587\u6587\u4ef6.c", []string{"src/\u4e2d\u6587*.c"}, true},
	} {
		if got := matchSVNFilePattern(tc.name, tc.patterns); got != tc.ok {
			t.Errorf("match(%q,%q)=%v want %v", tc.name, tc.patterns, got, tc.ok)
		}
	}
}
