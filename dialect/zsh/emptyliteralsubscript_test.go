// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// An array literal element whose subscript expanded to nothing, or to blanks,
// is read as zero here and refused the way `[0]=x` is — where the plain
// `a[$(true)]=x` is a math error. It was the math error on both roads (#6738).
//
// Measured 2026-10-16 on zsh 5.9.2 under `-c`, byte for byte; the script
// ends at each.
func TestAnEmptySubscriptInAnArrayLiteralIsZero(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"a=([$(true)]=x); echo after", "zsh:1: bad subscript for direct array assignment: "},
		{`a=([""]=x); echo after`, "zsh:1: bad subscript for direct array assignment: "},
		{`a=([$(echo " ")]=x); echo after`, "zsh:1: bad subscript for direct array assignment:  "},
		{"b=(1 [$(true)]=x 3); echo after", "zsh:1: bad subscript for direct array assignment: "},
		{"typeset -a q=([$(true)]=x); echo after", "zsh:1: bad subscript for direct array assignment: "},
		// The plain spelling keeps its own answer.
		{"a[$(true)]=x; echo after", "zsh:1: bad math expression: empty string"},
	} {
		out, _ := answersRun(t, c.src)
		if got := strings.TrimRight(out, "\n"); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// A store through an operand whose subscript lands out of range ends the
// script here, as the statement does — the answer
// Semantics.OutOfRangeElementToAnOutputOperand and its declaration sibling
// give this column. Measured 2026-10-16 on zsh 5.9.2 from a file; already
// the answer before #6835, which moved the other two columns.
func TestAnOutOfRangeStoreThroughABuiltinEndsTheScript(t *testing.T) {
	for _, src := range []string{
		"a=(1)\nread 'a[0]' <<< x; echo st=$?\necho next",
		"a=(1)\nprintf -v 'a[0]' x; echo st=$?\necho next",
		"a=(1)\ntypeset 'a[0]'=x; echo st=$?\necho next",
	} {
		out, _ := answersRun(t, src)
		if want := "zsh:2: a: assignment to invalid subscript range\n"; out != want {
			t.Errorf("%q\n got %q\nwant %q", src, out, want)
		}
	}
}
