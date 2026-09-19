// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration utility's operand may carry a subscript *and* the append
// operator, and then it joins that element rather than replacing it.
//
// It replaced it, silently and at status 0, which is the worst shape a wrong
// answer takes here: the array keeps its length, nothing is written to
// stderr, and an accumulate loop ends holding only its last iteration. The
// bare route (`a[1]+=v`, no utility word) was already right — see
// TestAppendingToAnArrayElement — so this is the declaration route alone, and
// a fix that only read the bare one would leave every `typeset` spelling wrong.
//
// Measured 2026-09-19 against bash 5.3.20, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`: `typeset -a a=(x y); typeset a[1]+=Q`
// leaves `a[1]` as `yQ`, where this shell left it `Q`.
func TestADeclarationOperandAppendsToItsElement(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"joins the element", `typeset -a a=(x y); typeset a[1]+=Q; echo "[${a[1]}]"`, "[yQ]\n"},
		// The array keeps its length, which is what makes the replacement
		// silent: a caller reading ${#a[@]} cannot tell the two apart.
		{"the array keeps its length", `typeset -a a=(x y); typeset a[1]+=Q; echo "[${#a[@]}]"`, "[2]\n"},
		// An element holding nothing yet joins to nothing rather than being
		// skipped, so the operator does not change which element is written.
		{"an empty element takes the value", `typeset -a a=(x); typeset a[1]+=Q; echo "[${a[1]}]"`, "[Q]\n"},
		// Without the operator the element is replaced, which is the control:
		// if this row and the first agree, the test is not reading the append.
		{"no operator still replaces", `typeset -a a=(x y); typeset a[1]=Q; echo "[${a[1]}]"`, "[Q]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runDeclareElement(t, tc.src, appendingElementSemantics)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The value an appending operand carries is an assignment's, so it is neither
// split on IFS nor matched against the filesystem.
//
// It was split: `v="p q"; typeset a[1]+=$v` joined only `p`, and the second
// word was dropped without a word of complaint. That is the same reading the
// scalar route already takes (#3772) — this asserts the subscripted operand
// reaches it too, which it did not.
//
// Measured 2026-09-19 against bash 5.3.20: the element holds `yp q`.
func TestAnAppendingElementOperandTakesAnAssignmentsExpansion(t *testing.T) {
	out, st := runDeclareElement(t,
		`typeset -a a=(x y); v="p q"; typeset a[1]+=$v; echo "[${a[1]}]"`,
		appendingElementSemantics)
	if out != "[yp q]\n" || st != 0 {
		t.Errorf("got %q (status %d), want the whole value joined unsplit at 0", out, st)
	}
}

// withSubscriptedAppendOperand answers the two axes this route asks — the
// append operator on a declaration's operand, and a subscript on one — in the
// one column that takes both, so the rows above read the join rather than a
// refusal.
func appendingElementSemantics(s *Semantics) {
	s.DeclarationTakesAnAppendOperand = Yes
	s.DeclarationTakesASubscriptedAppendOperand = Yes
}
