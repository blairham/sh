// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `((` is ambiguous, and the rule that settles it — the reading holds where
// the expression's own nesting is closed by two *adjacent* `)` — is unanimous.
// ArithCommandScanIgnoresQuoting is the narrower question underneath it:
// whether the scan looking for that closer sees a `)` written inside quotes.
//
// Measured 2026-09-15, each probe in a script file of its own. The three bash
// columns step over a quoted run whole, so the `)` inside `"a)b"` closes
// nothing, the `))` at the end is found and the line is an arithmetic command
// over an expression that fails. zsh 5.9.2 and ksh93u+ see that `)` at depth
// zero with `b` behind it, give the arithmetic reading up, and run two
// groupings that print `a)b` (#3069).
//
// Asserted on the tree rather than on a run, because both readings parse: the
// wrong one is an arithmetic command whose expression is bad at *run* time, so
// a test that asked only for a nil error would pass either.
func TestTheArithCommandScanCanBeBlindToQuoting(t *testing.T) {
	t.Parallel()
	blind := syntax.Core()
	blind.ArithCommandScanIgnoresQuoting = true

	for _, tc := range []struct {
		name, src string
		arith     bool // the core's reading, which tracks quoting
		blind     bool // the reading of a scan that does not
	}{
		{"a closer inside double quotes", "((echo \"a)b\"))", true, false},
		{"a closer inside single quotes", "((echo 'a)b'))", true, false},
		{"an opener inside double quotes", "((echo \"(\" ))", true, false},

		// The floor, and what keeps the flag about *quoting*. A backslash and
		// a command substitution are stepped over by every column in the
		// panel, so neither reading may move on these.
		{"an escaped closer", "((echo a\\) ))", true, true},
		{"a closer inside a command substitution", "((echo $(echo a) ))", true, true},

		// And an ordinary expression is an arithmetic command either way.
		{"an expression with no quoting in it", "((1 + 2))", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isArithCommand(t, tc.src, syntax.Core()); got != tc.arith {
				t.Errorf("%q with the scan tracking quoting is arithmetic = %v, want %v", tc.src, got, tc.arith)
			}
			if got := isArithCommand(t, tc.src, blind); got != tc.blind {
				t.Errorf("%q with the scan blind to quoting is arithmetic = %v, want %v", tc.src, got, tc.blind)
			}
		})
	}
}

// isArithCommand reports whether the only command of src is an arithmetic
// command rather than the groupings `((` also spells.
func isArithCommand(t *testing.T, src string, d syntax.Dialect) bool {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%q read %d statements, want 1", src, len(f.Stmts))
	}
	_, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.ArithCmdClause)
	return ok
}
