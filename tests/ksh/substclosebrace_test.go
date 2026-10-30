// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A `}` that begins a word inside a `$( )` body is a group's closer in
// ksh93u+, so with no group open in the body it is refused — when the
// substitution runs, at the status a syntax error has (#6960). Measured
// 2026-10-30 on ksh93u+ 2012-08-01 at /bin/ksh with `-c`, standard input on
// the null device; each row is that run's output and status.
func TestACloseBraceBeginningAWordInACommandSubstitutionIsRefused(t *testing.T) {
	const refused = "syntax error at line 1: `}' unexpected"
	for _, c := range []struct {
		src, out, err string
		status        int
	}{
		{"echo $(echo })", "", refused, 3},
		{"echo $(echo a })", "", refused, 3},
		{"echo $(echo }a)", "", refused, 3},
		{"echo $(echo a >})", "", refused, 3},
		{"echo $(case } in *) echo y;; esac)", "", refused, 3},
		{"echo $(for i in }; do echo $i; done)", "", refused, 3},
		{"echo $( [[ } == } ]] && echo y )", "", refused, 3},
		{"echo $( { echo a; }; echo } )", "", refused, 3},
		{"echo $( ( echo } ) )", "", refused, 3},
		{`echo "$(echo })"`, "", refused, 3},
		{"x=$(echo }); echo after", "", refused, 3},
		// When it runs: what stood before it has run, and a branch nothing
		// takes says nothing.
		{"echo before; echo $(echo }); echo after", "before\n", refused, 3},
		{"false && echo $(echo }); echo survived", "survived\n", "", 0},
		// Where it runs decides what it ends, as for any body refused there:
		// a substitution inside a substitution fails on its own and the
		// script goes on.
		{"echo $(echo $(echo })); echo st=$?", "", "", 0},
		// The controls: an open group, a function's body, a brace not at the
		// start of the word, and the two other spellings.
		{"echo $( { echo }; } )", "}\n", "", 0},
		{"echo $( { echo }a; } )", "}a\n", "", 0},
		{"echo $( f() { echo }; }; f )", "}\n", "", 0},
		{"echo $( { ( echo } ); } )", "}\n", "", 0},
		{"echo $(echo a})", "a}\n", "", 0},
		{`echo $(echo \})`, "}\n", "", 0},
		{"echo $(x=}; echo $x)", "}\n", "", 0},
		{"echo `echo }`", "}\n", "", 0},
		{"echo }", "}\n", "", 0},
	} {
		out, st := runKsh(t, t.TempDir(), c.src)
		bad := st != c.status
		switch {
		case c.err != "":
			bad = bad || !strings.HasPrefix(out, c.out) ||
				!strings.HasSuffix(strings.TrimSuffix(out, "\n"), c.err)
		case c.out != "":
			bad = bad || out != c.out
		default:
			// The nested row: the inner body's refusal and then the outer
			// echo's empty line and the status, which is still 0.
			bad = bad || !strings.Contains(out, refused) || !strings.HasSuffix(out, "\n\nst=0\n")
		}
		if bad {
			t.Errorf("%q: %q (status %d), want %q then %q at %d", c.src, out, st, c.out, c.err, c.status)
		}
	}
}
