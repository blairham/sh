// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestUnbalancedBracketsMakeTheDoubleParenSubshells pins that `((` and `$((`
// are arithmetic only where the square brackets balance at the `))`, and two
// subshells otherwise. Measured 2026-10-02 on zsh 5.9.2 under `-f` in an empty
// directory (#5378).
func TestUnbalancedBracketsMakeTheDoubleParenSubshells(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(( abcdefghijklmnop[1 )); echo after", "zsh:1: bad pattern: abcdefghijklmnop[1\nafter\n"},
		{"(( x + a[1 ))", "zsh:1: bad pattern: a[1\n"},
		{"(( 1 ] ))", "zsh:1: command not found: 1\n"},
		{"print $(( a[1 ))", "zsh:1: bad pattern: a[1\n\n"},
		{"print $(( 1 ] ))", "zsh:1: command not found: 1\n\n"},
		// The controls: balanced brackets stay arithmetic.
		{"a=(4 5); (( a[2] == 5 )) && print yes", "yes\n"},
		{"a=(4 5); print $(( a[2] ))", "5\n"},
		{"(( a[[1]] )); echo st=$?", "st=1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAnOperandWhereTheColonBelongedIsBlamedAsAnOperand pins the conditional's
// refusal when the then-branch is followed by an operand rather than a `:`.
// Measured 2026-10-02 on zsh 5.9.2 (#5378).
func TestAnOperandWhereTheColonBelongedIsBlamedAsAnOperand(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"(( 1 ? 2 abcdefghijklmn ))", "zsh:1: bad math expression: operator expected at `abcdefghij...'\n"},
		{"echo $(( 1 ? 2 (3) ))", "zsh:1: bad math expression: operator expected at `(3) '\n"},
		{"(( 1 ? 2 ; ))", "zsh:1: bad math expression: ':' expected\n"},
		{"(( 1 ? 2 ))", "zsh:1: bad math expression: ':' expected\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
