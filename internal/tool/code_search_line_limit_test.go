// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package tool

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBoundedSearchLine(t *testing.T) {
	const short = "int number = 123;"
	if got := boundedSearchLine(short); got != short {
		t.Fatalf("short line changed: %q", got)
	}
	long := strings.Repeat("a", 1024*1024)
	got := boundedSearchLine(long)
	if len(got) > maxCodeSearchLineBytes+len("... [line truncated]") ||
		!strings.HasSuffix(got, "... [line truncated]") {
		t.Fatalf("giant source line not bounded: length=%d", len(got))
	}
	multi := strings.Repeat("中文字符", 400)
	got = boundedSearchLine(multi)
	if !utf8.ValidString(got) || !strings.HasSuffix(got, "... [line truncated]") {
		t.Fatalf("multibyte source line truncated incorrectly: %q", got)
	}
}
