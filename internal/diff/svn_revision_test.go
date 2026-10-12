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

// The checkout deliberately contains uncommitted edits when testing history:
// revision reviews must read remote snapshots, not current disk contents.
func TestSVNRevisionProviders(t *testing.T) {
	if _, err := exec.LookPath("svnadmin"); err != nil {
		t.Skip("svnadmin is not installed")
	}
	if _, err := exec.LookPath("svn"); err != nil {
		t.Skip("svn is not installed")
	}

	repo := filepath.Join(t.TempDir(), "repository")
	wc := filepath.Join(t.TempDir(), "working-copy")
	runTestCommand(t, "", "svnadmin", "create", repo)
	runTestCommand(t, "", "svn", "checkout", "file://"+filepath.ToSlash(repo), wc)
	main := filepath.Join(wc, "main.c")
	old := filepath.Join(wc, "obsolete.c")
	write := func(path, val string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(val), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write(main, "int number = 1;\n")
	write(old, "old file\n")
	runTestCommand(t, wc, "svn", "add", "main.c", "obsolete.c")
	runTestCommand(t, wc, "svn", "commit", "-m", "revision 1")

	write(main, "int number = 2;\n")
	write(filepath.Join(wc, "space name.c"), "int spaced = 2;\n")
	runTestCommand(t, wc, "svn", "add", "space name.c")
	runTestCommand(t, wc, "svn", "commit", "-m", "revision 2")

	write(main, "int number = 3;\n")
	runTestCommand(t, wc, "svn", "delete", "obsolete.c")
	runTestCommand(t, wc, "svn", "commit", "-m", "revision 3")

	// A dirty working copy must not affect the historical snapshot.
	write(main, "LOCAL UNCOMMITTED FILE\n")
	ctx := context.Background()

	commit := NewSVNCommitProvider(wc, "2", nil)
	set, err := commit.GetDiffSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, d := range set.Included {
		seen[d.NewPath] = true
		if d.NewPath == "main.c" {
			if !strings.Contains(d.Diff, "+int number = 2;") || d.NewFileContent != "int number = 2;\n" {
				t.Errorf("incorrect historical file content: diff=%q content=%q", d.Diff, d.NewFileContent)
			}
		}
		if d.NewPath == "space name.c" && !d.IsNew {
			t.Errorf("added SVN file should be new: %+v", d)
		}
	}
	if !seen["main.c"] || !seen["space name.c"] {
		t.Fatalf("missing commit r2 paths: %+v", seen)
	}

	rng := NewSVNRangeProvider(wc, "1", "3", nil)
	diffs, err := rng.GetDiff(ctx)
	if err != nil {
		t.Fatal(err)
	}
	foundDeleted, foundChanged := false, false
	for _, d := range diffs {
		if d.NewPath == "/dev/null" && d.OldPath == "obsolete.c" && d.IsDeleted {
			foundDeleted = true
		}
		if d.NewPath == "main.c" && d.NewFileContent == "int number = 3;\n" {
			foundChanged = true
		}
	}
	if !foundDeleted || !foundChanged {
		t.Fatalf("incorrect SVN range r1:r3: deleted=%v changed=%v diffs=%+v", foundDeleted, foundChanged, diffs)
	}
	content, err := ReadSVNRevisionFile(ctx, wc, "main.c", "2")
	if err != nil || content != "int number = 2;\n" {
		t.Fatalf("historical svn cat = %q, %v", content, err)
	}
	for _, path := range []string{"../outside.c", "/tmp/outside.c"} {
		if _, err := ReadSVNRevisionFile(ctx, wc, path, "2"); err == nil {
			t.Errorf("expected invalid path error for %q", path)
		}
	}
}
