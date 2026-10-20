// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A bare assignment cannot begin either branch of a conditional in this
// shell, and the two branches refuse it with different sentences.
//
// Measured 2026-09-26 on zsh 5.9.2 (aarch64-apple-darwin25.4.0) run `-f`.
// The then branch says `':' expected`, because the `=` stands where the colon
// was looked for; the else branch says `lvalue required`, because there the
// whole conditional is left standing as the target of the store. The
// parenthesized rows are the controls that say this is a level and not a ban
// — the same assignment in the same position is taken — and the last two are
// the plain conditional and a nested one, which must not move (#4680).
func TestAConditionalBranchWillNotBeginWithAnAssignment(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{
			"the then branch", `print -- "$(( 1 ? x = 2 : 3 ))"`,
			"zsh:1: bad math expression: ':' expected\n", 1,
		},
		{
			"the then branch, compound", `print -- "$(( 0 ? x += 2 : 3 ))"`,
			"zsh:1: bad math expression: ':' expected\n", 1,
		},
		{
			"the else branch", `print -- "$(( 1 ? 2 : x = 3 ))"`,
			"zsh:1: bad math expression: lvalue required\n", 1,
		},
		{"parenthesized in the then", `print -- "$(( 1 ? (x = 2) : 3 ))"`, "2\n", 0},
		{"parenthesized in the else", `print -- "$(( 0 ? 2 : (x = 3) ))"`, "3\n", 0},
		{"an ordinary conditional", `print -- "$(( 1 ? 2 : 3 ))"`, "2\n", 0},
		{"nested in the then", `print -- "$(( 1 ? 0 ? 5 : 6 : 3 ))"`, "6\n", 0},
		{"nested in the else", `print -- "$(( 0 ? 1 : 0 ? 2 : 3 ))"`, "3\n", 0},
		{"an assignment around the whole of it", `print -- "$(( x = 1 ? 2 : 3 ))"`, "2\n", 0},

		// The then branch is below the sequence operator as well as below
		// assignment, which is what makes this column's then level its own
		// rather than the core's. Measured 2026-09-27 on 5.9.2 run `-f`:
		// `$(( 1 ? 2 , 3 : 4 ))` is `':' expected` here and 3 in bash 5.3.20,
		// ksh93u+ 2012-08-01 and BusyBox ash 1.37.0. The row under it is the
		// control that keeps it off the operator itself — a comma written
		// after the whole conditional is taken here exactly as it is there,
		// so the refusal is about *where* the comma stood (#4776).
		{
			"a comma in the then branch", `print -- "$(( 1 ? 2 , 3 : 4 ))"`,
			"zsh:1: bad math expression: ':' expected\n", 1,
		},
		{"a comma after the conditional", `print -- "$(( 1 ? 2 : 3 , 4 ))"`, "4\n", 0},
		{"the sequence operator on its own", `print -- "$(( 1 , 2 ))"`, "2\n", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
