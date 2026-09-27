// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"testing"

	"github.com/blairham/sh/syntax"
)

// The level the two branches of `c ? t : e` are read at.
//
// The flag is named here and the shell that sets it is not. It is a level and
// not a ban, which the parenthesized rows carry: the same store in the same
// position is taken under both readings, so what the flag moves is where a
// *bare* assignment may begin.
func TestAConditionalBranchIsReadAtTheDialectsLevel(t *testing.T) {
	t.Parallel()
	below := syntax.Core()
	below.ArithConditionalBranchBelowAssignment = true
	for _, tc := range []struct {
		name  string
		src   string
		below bool // refused when the branch is read below assignment
	}{
		{"a bare assignment in the then", `1 ? x = 2 : 3`, true},
		{"a bare compound assignment in the then", `0 ? x += 2 : 3`, true},
		{"a bare assignment in the else", `1 ? 2 : x = 3`, true},
		{"a parenthesized assignment in the then", `1 ? (x = 2) : 3`, false},
		{"a parenthesized assignment in the else", `1 ? 2 : (x = 3)`, false},
		{"an ordinary conditional", `1 ? 2 : 3`, false},
		{"a conditional inside the then", `1 ? 0 ? 5 : 6 : 3`, false},
		{"a conditional inside the else", `0 ? 1 : 0 ? 2 : 3`, false},
		{"an increment in the then", `1 ? x++ : 3`, false},
		{"an assignment around the whole of it", `x = 1 ? 2 : 3`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := arithErr(tc.src, syntax.Core()); err != nil {
				t.Errorf("read %q at the assignment level: %v, want no error", tc.src, err)
			}
			err := arithErr(tc.src, below)
			if tc.below && err == nil {
				t.Errorf("read %q below assignment: no error, want one", tc.src)
			}
			if !tc.below && err != nil {
				t.Errorf("read %q below assignment: %v, want no error", tc.src, err)
			}
		})
	}
}
