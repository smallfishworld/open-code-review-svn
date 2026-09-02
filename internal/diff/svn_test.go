// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package diff

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConvertSVNDiff(t *testing.T) {
	input := "Index: src/main.c\n===================================\n--- src/main.c\t(revision 1)\n+++ src/main.c\t(working copy)\n@@ -1 +1 @@\n-old\n+new\n"
	got := convertSVNDiff(input, map[string]string{"src/main.c": "modified"})
	for _, want := range []string{"diff --git a/src/main.c b/src/main.c", "-old", "+new"} {
		if !strings.Contains(got, want) {
			t.Errorf("converted diff does not contain %q:\n%s", want, got)
		}
	}
}

func TestConvertSVNDiffBinary(t *testing.T) {
	input := "Index: asset.bin\n===================================\nCannot display: file marked as a binary type.\nsvn:mime-type = application/octet-stream\n"
	got := convertSVNDiff(input, map[string]string{"asset.bin": "modified"})
	if !strings.Contains(got, "Binary files a/asset.bin and b/asset.bin differ") {
		t.Fatalf("binary marker missing:\n%s", got)
	}
}

func TestUnversionedFilesRecursesAndSorts(t *testing.T) {
	repo := t.TempDir()
	for _, name := range []string{"new-dir/z.c", "new-dir/nested/a.c", "single.c"} {
		path := filepath.Join(repo, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	provider := &Provider{repoDir: repo}
	got := provider.unversionedFiles(map[string]string{
		"new-dir":   "unversioned",
		"single.c":  "unversioned",
		"ignored.c": "ignored",
	})
	want := []string{"new-dir/nested/a.c", "new-dir/z.c", "single.c"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unversionedFiles = %v, want %v", got, want)
	}
}

func TestSVNWorkspaceProvider(t *testing.T) {
	if _, err := exec.LookPath("svnadmin"); err != nil {
		t.Skip("svnadmin is not installed")
	}
	repo := filepath.Join(t.TempDir(), "repo")
	wc := filepath.Join(t.TempDir(), "wc")
	runTestCommand(t, "", "svnadmin", "create", repo)
	runTestCommand(t, "", "svn", "checkout", "file://"+repo, wc)
	file := filepath.Join(wc, "main.c")
	if err := os.WriteFile(file, []byte("int value = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deletedFile := filepath.Join(wc, "deleted.c")
	if err := os.WriteFile(deletedFile, []byte("int deleted;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spacedFile := filepath.Join(wc, "space name.c")
	if err := os.WriteFile(spacedFile, []byte("int spaced = 1;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, wc, "svn", "add", "main.c", "deleted.c", "space name.c")
	runTestCommand(t, wc, "svn", "commit", "-m", "initial")
	if err := os.WriteFile(file, []byte("int value = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spacedFile, []byte("int spaced = 2;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wc, "new.c"), []byte("int added;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wc, "scheduled.c"), []byte("int scheduled;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nestedDir := filepath.Join(wc, "new-dir", "nested")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "child.c"), []byte("int child;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wc, "asset.bin"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, wc, "svn", "add", "scheduled.c")
	runTestCommand(t, wc, "svn", "delete", "deleted.c")

	provider := NewWorkspaceProvider(wc, nil)
	if !provider.svn {
		t.Fatal("expected SVN provider")
	}
	diffs, err := provider.GetDiff(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 7 {
		t.Fatalf("got %d diffs, want 7: %+v", len(diffs), diffs)
	}
	byPath := make(map[string]bool, len(diffs))
	for _, diff := range diffs {
		byPath[diff.NewPath] = diff.IsBinary
	}
	if _, ok := byPath["new-dir/nested/child.c"]; !ok {
		t.Error("nested unversioned file was not included")
	}
	if !byPath["asset.bin"] {
		t.Error("unversioned binary file was not marked binary")
	}
	if _, ok := byPath["space name.c"]; !ok {
		t.Error("path containing spaces was not included")
	}
}

func runTestCommand(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s failed: %v: %s", name, err, out)
	}
}
