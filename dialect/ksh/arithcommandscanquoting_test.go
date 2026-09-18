// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The scan that looks for an arithmetic command's `))` is blind to quoting
// here, so a `)` written inside quotes closes the expression's nesting, the
// arithmetic reading is given up, and two groupings run the command.
//
// Measured 2026-09-15 on ksh93u+ 2012, each probe in a script file of its own:
//
//	((echo "a)b"))      prints a)b, 0
//	((echo 'a)b'))      prints a)b, 0
//	((echo "(" ))       `"' unmatched, 3
//	((echo a\) ))       arithmetic syntax error, 1
//	((echo $(echo a) )) arithmetic syntax error, 1
//
// bash 5.3, bash 3.2 and bash-as-`sh` call each of the first three an
// arithmetic syntax error at 1. The last two are the floor and keep the flag
// about *quoting*: a backslash and a command substitution are stepped over in
// every column, so those two rows do not move.
//
// The third row is left standing, and deliberately. The flag alone leaves two
// groupings that print `(`, which is what zsh 5.9.2 does with the same text;
// this shell instead meets a quote it cannot close, having given the reading
// up *at* the `(` inside the quotes rather than starting again from the front.
// That is a fact about where a fallback resumes and not about whether the scan
// sees quoting, so it is not this flag's to answer (#3069).
func TestTheArithCommandScanIsBlindToQuoting(t *testing.T) {
	if !ksh.Dialect().ArithCommandScanIgnoresQuoting {
		t.Fatal("zsh's arithmetic-command scan does not see quoting")
	}
	for _, tc := range []struct{ src, want string }{
		{"((echo \"a)b\"))", "a)b\n"},
		{"((echo 'a)b'))", "a)b\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, tc.want)
		}
	}
	// The floor: still an arithmetic command, so still a run-time complaint
	// about the expression rather than a line that prints.
	for _, src := range []string{"((echo a\\) ))", "((echo $(echo a) ))"} {
		out, st, _ := preset.Combined(t, dialecttest.Base{}, src)
		if st == 0 {
			t.Errorf("%q ran at 0 printing %q, want the expression refused", src, out)
		}
	}
}
