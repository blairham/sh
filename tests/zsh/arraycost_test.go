// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"testing"
)

// The cost of reading one element, counting, and appending does not grow with
// the array (#6099).
//
// Each row runs the same operation on an array of 2,000 elements and on one of
// 20,000 and compares what one run of it allocates; see
// [dialecttest.Preset.CheckArrayOperationsDoNotGrow]. An operation that reads
// the array into a list allocates at least a string header per element, and one
// that does not allocates the same on both. It was a ratio of two wall-clock
// times until #6411: each about a millisecond, which a shared runner's jitter
// exceeds, and it failed the macOS job three times on diffs that could not
// reach it.
//
// Every row walked the array before #6099: the store was a map searched for
// its end on every append, a read built the whole list to index it, and a
// count built the list, or the join, to measure it. The element read inside
// an expression still built the list after that, and so did the length of one
// element on an array with a gap (#6358); those are the last two rows.
//
// `${a[*]}` is the control: a join reads every element, and the check fails
// outright if it is not seen to grow.
func TestArrayOperationsDoNotGrowWithTheArray(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two large arrays per row")
	}
	build := func(n int) string {
		return `a=(); for ((i=0;i<` + strconv.Itoa(n) + `;i++)); do a+=(x); done`
	}
	preset.CheckArrayOperationsDoNotGrow(t, build, `y="${a[*]}"`,
		`y=${a[5]}`, `y=$#a`, `y=${#a[@]}`, `a+=(z)`, `y=$(( a[5] ))`, `y=${#a[5]}`)
}

// The counts the faster paths answer are the ones the reference gives, in the
// shapes those paths decline as well as the ones they take: an empty array, a
// gap, a local shadowing a global, a tied pair, `nounset`, and an unset name.
// Measured on zsh 5.9.2, 2026-10-05.
func TestCountingAnArrayAnswersAsTheReferenceDoes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`a=(x y z); print $#a ${#a} ${#a[@]} ${#a[*]}`, "3 3 3 3\n"},
		{`a=(); print $#a ${#a} ${#a[@]}`, "0 0 0\n"},
		{`a=(x); a[5]=y; print $#a ${#a} ${#a[@]}`, "5 5 5\n"},
		{`setopt nounset; a=(p q); print $#a ${#a[@]}`, "2 2\n"},
		{`a=(p q); unset a; print ${#a} ${#a[@]}`, "0 0\n"},
		{`a=(p q); f() { local a=(1 2 3); print $#a; }; f; print $#a`, "3\n2\n"},
		{`a=("x y" z); print ${#a} "${#a}" ${#a[2]} ${#a[1]}`, "2 2 1 3\n"},
		{`typeset -T FOO foo; foo=(a b c); print $#foo $#FOO`, "3 5\n"},
		{`IFS=:; a=(ab cd); print $#a ${#a[@]}`, "2 2\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}
