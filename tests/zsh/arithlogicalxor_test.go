// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `^^` is this shell's logical exclusive-or: 1 where exactly one operand is
// true, the same 1-or-0 result every other logical operator yields. No script
// that runs under bash can contain one, and the spelling was already taken
// here — `^` is bitwise xor — so the diagnostic blamed a missing operand
// rather than the operator not existing.
//
// Measured 2026-09-18 against zsh 5.9.2, `env -i` with LC_ALL=C over a script
// file (#2991).
func TestTheLogicalExclusiveOr(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"1 ^^ 1", "0"},
		{"1 ^^ 0", "1"},
		{"0 ^^ 1", "1"},
		{"0 ^^ 0", "0"},
		// The truth of the operands and not their value, which is what makes
		// it logical rather than bitwise: `5 ^^ 3` is 0 where `5 ^ 3` is 6.
		{"2 ^^ 3", "0"},
		{"2 ^^ 0", "1"},
		{"5 ^^ 3", "0"},
		{"-1 ^^ 0", "1"},
		{"1 ^ 1", "0"},
		{"5 ^ 3", "6"},
		// Truth is a property of the value and not of its kind.
		{"1.5 ^^ 2.5", "0"},
		{"1.5 ^^ 0", "1"},
		// It shares the `||` rung here, left-associative, and `&&` binds
		// tighter than both.
		{"1 || 0 ^^ 1", "0"},
		{"0 ^^ 1 || 1", "1"},
		{"1 ^^ 1 || 1", "1"},
		{"1 || 1 ^^ 1", "0"},
		{"1 && 0 ^^ 1", "1"},
		{"1 ^^ 0 && 0", "1"},
		{"1 ^^ 1 ^^ 1", "1"},
	} {
		out, st := answersRun(t, `echo $(( `+tc.src+` ))`)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("$(( %s )): got %q at %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}

// Both operands are evaluated, because neither side can decide the answer
// alone — there is nothing to short-circuit. Measured the same day: the
// assignment in the right operand happens whichever way the left one went.
func TestTheLogicalExclusiveOrEvaluatesBothOperands(t *testing.T) {
	for _, left := range []string{"0", "1"} {
		out, st := answersRun(t, `echo $(( `+left+` ^^ (y=9) )); echo y=$y`)
		if !strings.Contains(out, "y=9") || st != 0 {
			t.Errorf("left %s: got %q at %d, want the right operand evaluated", left, out, st)
		}
	}
}

// And the assignment spelling beside it, which is the one compound assignment
// here that assigns nothing: `x=5; $(( x ^^= 1 ))` writes 0 and leaves `x` at
// 5, where `||=` and `&&=` in the same manual both store. Measured 2026-09-18.
func TestTheLogicalExclusiveOrAssignmentStoresNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"x=5; echo $(( x ^^= 1 )); echo x=$x", "0\nx=5"},
		{"x=0; echo $(( x ^^= 1 )); echo x=$x", "1\nx=0"},
		{"x=1; y=2; echo $(( x ^^= y ))", "0"},
		// It is looser than `||`, the way an assignment operator is: the
		// whole of what follows is its value.
		{"x=1; echo $(( x ^^= 1 || 1 ))", "0"},
		// And where the left side cannot be a target it is still the
		// exclusive-or of what precedes it.
		{"x=0; echo $(( 1 || x ^^= 1 ))", "0"},
		// The control: the bitwise spelling does store.
		{"x=5; echo $(( x ^= 1 )); echo x=$x", "4\nx=4"},
	} {
		out, st := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || st != 0 {
			t.Errorf("%s: got %q at %d, want %q at 0", tc.src, got, st, tc.want)
		}
	}
}
