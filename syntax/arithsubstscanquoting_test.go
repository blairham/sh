// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// `$((` is ambiguous the way `((` is, and the rule that settles it is the same
// one: counting from just inside, the construct is arithmetic when the `)`
// that brings the count back to zero has another `)` right behind it.
// ArithSubstScanIgnoresQuoting is the narrower question underneath — whether
// the scan looking for that closer sees a `)` written inside quotes — and it
// is a second field rather than the one the `((` scan reads, because the two
// were measured separately and agree today by measurement rather than by
// construction (#3530, #3069).
//
// Measured 2026-09-18, each probe in a script file of its own. A scan that
// tracks quoting steps over `'0)'` whole, finds the `))` and reads arithmetic;
// a scan that does not meets that `)` at depth zero with ` + 1 ` behind it,
// gives the arithmetic reading up and leaves a command substitution holding a
// subshell — which runs a command named `0)`.
//
// Asserted on the tree rather than on a run, because both readings parse.
func TestTheArithSubstScanCanBeBlindToQuoting(t *testing.T) {
	t.Parallel()
	tracks := syntax.Core()
	tracks.ArithSubstFallsBackToCommandSubst = true
	blind := tracks
	blind.ArithSubstScanIgnoresQuoting = true

	for _, tc := range []struct {
		name, src     string
		arith, blindA bool
	}{
		{"a closer inside single quotes", `echo $(( '0)' + 1 ))`, true, false},
		{"a closer inside double quotes", `echo $(( "0)" + 1 ))`, true, false},

		// The floor, and what keeps the field about *quoting*: a backslash is
		// stepped over under either reading.
		{"an escaped closer", `echo $(( 1\) + 1 ))`, true, true},
		// And an ordinary expression is arithmetic either way.
		{"an expression with no quoting in it", `echo $(( 1 + 2 ))`, true, true},
		// A body that is genuinely a subshell is a command substitution
		// under both, which is what says neither reading is a blanket.
		{"a subshell that closes once", `echo $( (echo x) )`, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isArithSubst(t, tc.src, tracks); got != tc.arith {
				t.Errorf("%q with the scan tracking quoting is arithmetic = %v, want %v", tc.src, got, tc.arith)
			}
			if got := isArithSubst(t, tc.src, blind); got != tc.blindA {
				t.Errorf("%q with the scan blind to quoting is arithmetic = %v, want %v", tc.src, got, tc.blindA)
			}
		})
	}
}

// isArithSubst reports whether the word of `echo …` holds an arithmetic
// substitution rather than the command substitution `$((` also spells.
func isArithSubst(t *testing.T, src string, d syntax.Dialect) bool {
	t.Helper()
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("%q: %v", src, err)
	}
	if len(f.Stmts) != 1 {
		t.Fatalf("%q read %d statements, want 1", src, len(f.Stmts))
	}
	cmd, ok := f.Stmts[0].Expr.(*syntax.Pipeline).Cmds[0].(*syntax.SimpleCmd)
	if !ok || len(cmd.Args) != 2 {
		t.Fatalf("%q did not read as one command with one operand", src)
	}
	for _, span := range cmd.Args[1].Spans {
		if span.Kind == syntax.ArithSubst {
			return true
		}
	}
	return false
}
