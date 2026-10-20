// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `let` given an expression whose reader met a byte it refuses. Two answers of
// this shell's own, measured against zsh 5.9.2 on 2026-09-11.

// The reader stops at the byte and what it had by then stands, which `let`
// then reads for truth. The pair differing only in the digit is the
// measurement: both fail identically and part company over the value.
func TestLetKeepsWhatStoodBeforeARefusedByte(t *testing.T) {
	for _, tc := range []struct {
		src  string
		want int
	}{
		{`let '1 @'`, 0},
		{`let '2 @'`, 0},
		{`let '1+2 @'`, 0},
		{`let '5 }'`, 0},
		{`let '1 @ 0'`, 0},
		{`let '0 @'`, 1},
		{`let '0 @ 5'`, 1},
		// Nothing stood before it.
		{`let '@'`, 1},
		// Not any math failure: a value stood before each of these too.
		{`let '1+'`, 1},
		{`let '5 5'`, 1},
		{`let '1/0'`, 1},
		// And the expressions after the failing one are not reached.
		{`let '0 @' '3'`, 1},
		{`let '3' '0 @'`, 1},
	} {
		if _, st := answersRun(t, tc.src); st != tc.want {
			t.Errorf("%s: status %d, want %d", tc.src, st, tc.want)
		}
	}
}

// A math complaint is the shell's own here rather than the builtin's, so it
// carries no builtin in the location — where this shell's rule is to put one
// there for every builtin that speaks.
//
// `let` with no operand is the control: that one *is* the builtin's, and does
// name it.
func TestAMathComplaintFromLetNamesNoBuiltin(t *testing.T) {
	for _, src := range []string{`let '1+'`, `let '1/0'`, `let '1 @'`} {
		out, _ := answersRun(t, src)
		if strings.Contains(out, ":let:") {
			t.Errorf("%s: got %q, want no builtin in the location", src, out)
		}
		if !strings.HasPrefix(strings.TrimSpace(out), "zsh:1:") {
			t.Errorf("%s: got %q, want the shell's own location", src, out)
		}
	}
	out, _ := answersRun(t, `let`)
	if !strings.Contains(out, ":let:") {
		t.Errorf("no operand: got %q, want the builtin named", out)
	}
}
