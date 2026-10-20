// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// The character constant's escapes are this shell's own, and `\C` spans
// differently here from the way the other shell with a character-code
// operator spans it: one argument, **no dash in the spelling**, and `\M-` a
// complete escape that takes none.
//
// Measured 2026-09-26 on ksh93u+ 2012-08-01. The rows that refuse are the
// discriminating ones: the escape ends before the quote does, so a character
// is left standing where an operator belongs, and a span that reached one
// byte further would answer a number at status 0 instead (#4607).
func TestTheCharacterConstantReadsThisShellsCaretEscapes(t *testing.T) {
	for _, tc := range []struct{ name, expr, want string }{
		{"no dash, lowercase", `'\Ca'`, "1\n"},
		{"no dash, uppercase", `'\CA'`, "1\n"},
		{"the dash is the argument", `'\C-'`, "109\n"},
		{"a letter as the argument", `'\C'`, "103\n"},
		{"an ordinary character", `'a'`, "97\n"},
		{"an octal escape is unchanged", `'\101'`, "65\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := arithLine(t, tc.expr); got != tc.want {
				t.Errorf("$((%s)): got %q, want %q", tc.expr, got, tc.want)
			}
		})
	}
	for _, tc := range []struct{ name, expr string }{
		{"a dash and a letter", `'\C-a'`},
		{"meta with a letter behind it", `'\M-x'`},
		{"meta without a dash", `'\Mx'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := arithLine(t, tc.expr)
			want := arithLoc + tc.expr + ": arithmetic syntax error\n"
			if got != want {
				t.Errorf("$((%s)): got %q, want %q", tc.expr, got, want)
			}
		})
	}
}
