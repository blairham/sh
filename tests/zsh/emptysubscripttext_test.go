// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// A subscript whose text expanded to nothing is not read as an expression in
// this shell at all, and the two emptinesses are two sentences.
//
// Measured 2026-09-11 on zsh 5.9.2, each row a `-c` of its own with
// `a=(5 6 7)` and `w=`. The written `${a[]}` one construct over is a third
// sentence again — `invalid subscript` — which is why they are two axes.
func TestASubscriptThatExpandedToNothingIsAMathError(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(5 6 7); w=; echo "[${a[$w]}]"`, "bad math expression: empty string"},
		{`s=str; w=; echo "[${s[$w]}]"`, "bad math expression: empty string"},
		{`w=; echo "[${nosucharr[$w]}]"`, "bad math expression: empty string"},
		{`a=(5 6 7); w=; echo "[${#a[$w]}]"`, "bad math expression: empty string"},
		{`a=(5 6 7); w=; a[$w]=z`, "bad math expression: empty string"},
		{`a=(5 6 7); echo "[${a[ ]}]"`, "bad math expression: operand expected at end of string"},
		{`a=(5 6 7); w="  "; echo "[${a[$w]}]"`, "bad math expression: operand expected at end of string"},
	} {
		out, status := answersRun(t, tc.src+`; echo after`)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, "after") || status == 0 {
			t.Errorf("%s = %q (status %d), want the input to end", tc.src, out, status)
		}
	}
}

// And the three neighbors that keep their answers: a key is not an
// expression, a substring's offset that expanded to nothing is zero, and a
// subscript with something in it is read as it always was.
func TestWhatAnEmptySubscriptTextDoesNotReachHere(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -A m; m[k]=v; w=; echo "[${m[$w]}]"`, "[]"},
		{`x=abcdef; w=; echo "[${x:$w:2}]"`, "[ab]"},
		{`a=(5 6 7); echo "[${a[ 2 ]}]"`, "[6]"},
	} {
		out, status := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
		}
	}
}
