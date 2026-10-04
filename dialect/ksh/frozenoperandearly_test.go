// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A declaration's value over a frozen name is refused ahead of the command
// here — the command's own redirection never applies to the sentence — and
// the script ends. Measured 2026-10-03 on ksh93u+ 2012-08-01; see
// Semantics.OperandValueOverAFrozenNameRefusedFirst.
func TestADeclarationsValueOverAFrozenNameIsRefusedFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"readonly q=1; typeset q=4 2>/dev/null; echo st=$?", "q: is read only\n"},
		{"readonly q=1; typeset -i q=4 2>/dev/null; echo st=$?", "q: is read only\n"},
		{"readonly q=1; export q=4 >/dev/null 2>&1; echo st=$?", "q: is read only\n"},
		{"readonly q=1; typeset q=(b) 2>/dev/null; echo st=$?", "q: is read only\n"},
		// No value, no refusal.
		{"readonly q=1; typeset -i q 2>/dev/null; echo st=$?", "st=0\n"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("= %q, want it to end %q", out, tc.want)
			}
		})
	}
}
