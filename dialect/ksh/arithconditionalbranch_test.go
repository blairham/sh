// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// A bare assignment may begin a conditional's branch here, which is C's
// reading of the same grammar.
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
