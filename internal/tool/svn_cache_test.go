// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSVNReaderCachesWorkspaceContentPerRun(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.c")
	if err := os.WriteFile(path, []byte("first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fr := &FileReader{RepoDir: root, Mode: ModeWorkspace, SVN: true}
	first, err := fr.Read(context.Background(), "sample.c")
	if err != nil || first != "first\n" {
		t.Fatalf("initial content %q, %v", first, err)
	}
	if err := os.WriteFile(path, []byte("second\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cached, err := fr.Read(context.Background(), "sample.c")
	if err != nil || cached != first {
		t.Fatalf("cached content %q, %v", cached, err)
	}
	fresh, err := (&FileReader{RepoDir: root, Mode: ModeWorkspace, SVN: true}).Read(context.Background(), "sample.c")
	if err != nil || fresh != "second\n" {
		t.Fatalf("new review content %q, %v", fresh, err)
	}
}
