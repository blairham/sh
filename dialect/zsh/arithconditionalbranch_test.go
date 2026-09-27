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
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != tc.st {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
