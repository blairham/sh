// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestARangeReadsItsBackslashesAsTheWordDoes pins ksh93's reading of a
// backslash in a substring range: removed where the expansion is unquoted,
// kept before an ordinary character inside double quotes (#5637). Measured
// 2026-10-03 on ksh93u+ 2012-08-01.
func TestARangeReadsItsBackslashesAsTheWordDoes(t *testing.T) {
	if got, _ := runKsh(t, t.TempDir(), `foo=abc; echo ${foo:0:\1}`); got != "a\n" {
		t.Errorf("unquoted: got %q, want %q", got, "a\n")
	}
	if got, _ := runKsh(t, t.TempDir(), `foo=abc; echo "${foo:0:\1}"`); !strings.Contains(got, "arithmetic syntax error") {
		t.Errorf("quoted: got %q, want an arithmetic syntax error", got)
	}
}
