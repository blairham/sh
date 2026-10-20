// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// A bare assignment may begin a conditional's **then** branch here, and the
// sequence operator may stand in it, because this column reads that branch at
// C's `expression` level.
//
// This is the control half of #4680: one shell in the panel reads both
// branches at the conditional level and refuses a bare store there, and these
// rows are what says the other columns must not move with it. Measured
// 2026-09-26 — bash 5.3.20(1)-release and ksh93u+ 2012-08-01 both answer 2 to
// `$(( 1 ? x = 2 : 3 ))`, where zsh 5.9.2 calls it `':' expected`.
//
// The comma rows were measured 2026-09-27 on 5.3.20 and on the 3.2.57 macOS
// ships, which agree: `$(( 1 ? 2 , 3 : 4 ))` is 3, and a comma written after
// the whole conditional belongs to the expression around it, so
// `$(( 1 ? 2 : 3 , 4 ))` is 4 whichever level the else branch is read at.
// That pair is what makes the first row a statement about the *then* branch
// rather than about the comma (#4776).
func TestAConditionalThenBranchIsReadAtTheSequenceLevel(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"the then branch", `echo "$(( 1 ? x = 2 : 3 ))" "$x"`, "2 2\n"},
		{"the then branch, compound", `y=1; echo "$(( 1 ? y += 2 : 3 ))" "$y"`, "3 3\n"},
		{"not taken, and nothing stored", `echo "$(( 0 ? z = 2 : 3 ))" "[$z]"`, "3 []\n"},
		{"parenthesized", `echo "$(( 1 ? (w = 2) : 3 ))"`, "2\n"},
		{"an ordinary conditional", `echo "$(( 1 ? 2 : 3 ))"`, "2\n"},
		{"a comma in the then branch", `echo "$(( 1 ? 2 , 3 : 4 ))"`, "3\n"},
		{"a comma after the conditional", `echo "$(( 1 ? 2 : 3 , 4 ))"`, "4\n"},
		{"both", `echo "$(( 1 ? 2 , 3 : 4 , 5 ))"`, "5\n"},
		{"the then branch is not taken, so its comma is not run", `v=5; echo "$(( 0 ? (v = 1) , 3 : 4 ))" "$v"`, "4 5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the **else** branch is read below assignment, so a bare store cannot
// begin one: the whole conditional is left standing as the target of the `=`,
// which is not a place, and the refusal is the one `$(( 7 = 4 ))` earns.
//
// Measured 2026-09-27 on 5.3.20 and 3.2.57 alike, so it is not a version line;
// ksh93u+ 2012-08-01 answers 2 and 3 to the first two rows, which is why this
// is not a shared refusal and why the field is per branch (#4775).
//
// The last two rows are the controls that make it a level rather than a ban —
// a parenthesized store in the same position is taken, and the plain
// conditional does not move.
func TestAConditionalElseBranchWillNotBeginWithAnAssignment(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{
			"the branch taken", `echo "$(( 1 ? 2 : x = 3 ))"`,
			"sh: line 1: 1 ? 2 : x = 3 : attempted assignment to non-variable (error token is \"= 3 \")\n", 1,
		},
		{
			"the branch not taken", `echo "$(( 0 ? 2 : x = 3 ))"`,
			"sh: line 1: 0 ? 2 : x = 3 : attempted assignment to non-variable (error token is \"= 3 \")\n", 1,
		},
		{
			"the same refusal without a conditional at all", `echo "$(( 7 = 4 ))"`,
			"sh: line 1: 7 = 4 : attempted assignment to non-variable (error token is \"= 4 \")\n", 1,
		},
		{"parenthesized in the else", `echo "$(( 0 ? 2 : (x = 3) ))" "$x"`, "3 3\n", 0},
		{"an ordinary conditional", `echo "$(( 1 ? 2 : 3 ))"`, "2\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
