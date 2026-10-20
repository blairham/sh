// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strconv"
	"testing"
)

// Reading one element, its length, and one element inside an expression costs
// the same on a large array as on a small one (#6358).
//
// The array is built the way a ksh script builds one, `a=()` and then `a+=(7)`,
// and in this dialect that leaves position 0 empty and fills from 1: measured
// on ksh93u+, `a=(); a+=(x); typeset -p a` is `typeset -a a=([1]=x)`. So this
// is an array with a gap, which the store's fast path for positions 0 to n-1
// does not cover, and both `${#a[i]}` and `$(( a[i] ))` read the whole array
// into a list to take one element of it — quadratic over a loop.
//
// The measurement is dialect/zsh's TestArrayOperationsDoNotGrowWithTheArray,
// through the same helper: what one operation allocates on 2,000 elements and
// on 20,000, with `${a[*]}` as the control that must be seen to grow.
func TestArrayReadsDoNotGrowWithTheArray(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two large arrays per row")
	}
	build := func(n int) string {
		return `a=(); for ((i=0;i<` + strconv.Itoa(n) + `;i++)); do a+=(7); done`
	}
	preset.CheckArrayOperationsDoNotGrow(t, build, `y="${a[*]}"`,
		`y=${a[5]}`, `y=${#a[5]}`, `y=$(( a[5] ))`)
}

// The reads the faster path answers are the ones ksh93u+ gives, on the shapes
// that path now takes: the gap `a=()` leaves at 0, a gap unset in the middle,
// a gap written past the end, one past the end, and a negative subscript.
// Measured on ksh93u+ 2012-08-01, 2026-10-11.
func TestReadingAnElementOfAnArrayWithAGapAnswersAsTheReferenceDoes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(); a+=(p); a+=(qq); print "${a[0]-u}" ${a[1]} ${#a[2]} $((${#a[1]} + ${#a[2]}))`, "u p 2 3\n"},
		{`a=(10 20 30); unset 'a[1]'; print "${a[1]-u}" ${#a[1]} $((a[1])) $((a[2])) $((a[-1]))`, "u 0 0 30 30\n"},
		{`a=(10 20 30); a[7]=70; print $((a[7])) $((a[5])) $((a[-1])) ${#a[7]}`, "70 0 70 2\n"},
		{`a=(1 2); print $((a[5])) ${#a[5]}`, "0 0\n"},
	} {
		if out, st := runKsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}
