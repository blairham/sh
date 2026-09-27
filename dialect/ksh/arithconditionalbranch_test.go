// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// A bare assignment may begin either of a conditional's branches here, which
// is the one column in the panel that takes one in the **else**.
//
// This is the control half of #4680: one shell in the panel reads both
// branches at the conditional level and refuses a bare store there, and these
// rows are what says the other columns must not move with it. Measured
// 2026-09-26 — bash 5.3.20(1)-release and ksh93u+ 2012-08-01 both answer 2 to
// `$(( 1 ? x = 2 : 3 ))`, where zsh 5.9.2 calls it `':' expected`.
func TestAConditionalBranchTakesABareAssignment(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the then branch", `print -- "$(( 1 ? x = 2 : 3 ))" "$x"`, "2 2\n"},
		{"the then branch, compound", `y=1; print -- "$(( 1 ? y += 2 : 3 ))" "$y"`, "3 3\n"},
		{"not taken, and nothing stored", `print -- "$(( 0 ? z = 2 : 3 ))" "[$z]"`, "3 []\n"},
		{"parenthesized", `print -- "$(( 1 ? (w = 2) : 3 ))"`, "2\n"},
		{"an ordinary conditional", `print -- "$(( 1 ? 2 : 3 ))"`, "2\n"},

		// The else branch, which is this column alone. Measured 2026-09-27
		// on ksh93u+ 2012-08-01: `$(( 1 ? 2 : x = 3 ))` is 2 and
		// `$(( 0 ? 2 : x = 3 ))` is 3, where bash 5.3.20 and 3.2.57 refuse
		// both with `attempted assignment to non-variable`, zsh 5.9.2 with
		// `lvalue required`, and dash 0.5.12 and BusyBox ash 1.37.0 with an
		// arithmetic syntax error. The third row is what says the branch is
		// *read* at that level rather than stored into eagerly: the store in
		// the arm nobody takes does not happen (#4775).
		{"the else branch", `print -- "$(( 1 ? 2 : p = 3 ))"`, "2\n"},
		{"the else branch, taken", `print -- "$(( 0 ? 2 : q = 3 ))" "$q"`, "3 3\n"},
		{"the else branch, not taken and nothing stored", `r=9; print -- "$(( 1 ? 2 : r = 3 ))" "$r"`, "2 9\n"},

		// And the then branch is the sequence level, which this column shares
		// with bash and BusyBox ash. Measured the same day: `$(( 1 ? 2 , 3 :
		// 4 ))` is 3 here and `':' expected` in zsh. The second row is the
		// control that keeps it off the comma itself — a comma written after
		// the whole conditional belongs to the expression around it (#4776).
		{"a comma in the then branch", `print -- "$(( 1 ? 2 , 3 : 4 ))"`, "3\n"},
		{"a comma after the conditional", `print -- "$(( 1 ? 2 : 3 , 4 ))"`, "4\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
