// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// TestARangeReadsItsBackslashesAsTheWordDoes pins BusyBox ash's reading of a
// backslash in a substring range, which is ksh93's (#5637). Measured
// 2026-10-03 on BusyBox ash 1.37.0 in the pinned alpine image.
func TestARangeReadsItsBackslashesAsTheWordDoes(t *testing.T) {
	if got, _ := run(t, `foo=abc; echo ${foo:0:\1}`); got != "a\n" {
		t.Errorf("unquoted: got %q, want %q", got, "a\n")
	}
	if got, _ := run(t, `foo=abc; (echo "${foo:0:\1}"); echo st=$?`); !strings.Contains(got, "arithmetic syntax error") || !strings.HasSuffix(got, "st=2\n") {
		t.Errorf("quoted: got %q, want an arithmetic syntax error at 2", got)
	}
}
