// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// A bare assignment may begin a conditional's **then** branch here and may not
// begin its **else** branch, which is C's reading of the grammar POSIX defers
// arithmetic expansion to.
//
// Measured 2026-09-27 against dash 0.5.12: `$(( 0 ? x = 2 : 3 ))` is 3 with
// nothing stored and `$(( 1 ? x = 2 : 3 ))` is 2, while both
// `$(( 1 ? 2 : x = 3 ))` and `$(( 0 ? 2 : x = 3 ))` are `arithmetic
// expression: expecting EOF` at status 2 — byte for byte the sentence
// `$(( 7 = 4 ))` earns, which is the point: the whole conditional is left
// standing as the target of the store, and a store to a non-place is what this
// shell already refuses (#4775).
//
// This column cannot say anything about the **then** level beyond assignment,
// because the sequence operator is absent from it altogether: `$(( 1 , 2 ))`
// is the same refusal. That is recorded rather than guessed at — see
// syntax.Dialect.ArithConditionalThenLevel.
func TestAConditionalElseBranchWillNotBeginWithAnAssignment(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
		st              int
	}{
		{"the then branch", `echo "$(( 1 ? x = 2 : 3 ))" "$x"`, "2 2", 0},
		{"the then branch, not taken", `echo "$(( 0 ? z = 2 : 3 ))" "[$z]"`, "3 []", 0},
		{"the else branch", `echo "$(( 1 ? 2 : y = 3 ))"`, "expecting EOF", 2},
		{"the else branch, taken", `echo "$(( 0 ? 2 : y = 3 ))"`, "expecting EOF", 2},
		{"the same refusal with no conditional", `echo "$(( 7 = 4 ))"`, "expecting EOF", 2},
		{"parenthesized in the else", `echo "$(( 0 ? 2 : (w = 3) ))" "$w"`, "3 3", 0},
		{"an ordinary conditional", `echo "$(( 1 ? 2 : 3 ))"`, "2", 0},
		{"the sequence operator is absent here", `echo "$(( 1 , 2 ))"`, "expecting EOF", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.Contains(out, tc.want) || st != tc.st {
				t.Errorf("%s gave %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.st)
			}
		})
	}
}
