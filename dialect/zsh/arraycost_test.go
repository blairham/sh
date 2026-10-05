// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"testing"
)

// The cost of reading one element, counting, and appending does not grow with
// the array (#6099).
//
// Each row times the same 500 operations on an array of 2,000 elements and on
// one of 20,000, with the shell's own clock around the loop so that building
// the array is not in the figure. An operation that walks the array costs ten
// times as much on the larger one; one that does not costs the same. The
// threshold of 3 sits between the two with room for a loaded machine, and the
// best of three runs is taken so that one descheduled run does not decide it.
//
// Every row walked the array before #6099: the store was a map searched for
// its end on every append, a read built the whole list to index it, and a
// count built the list, or the join, to measure it.
func TestArrayOperationsDoNotGrowWithTheArray(t *testing.T) {
	if testing.Short() {
		t.Skip("times loops over large arrays")
	}
	for _, op := range []string{`y=${a[5]}`, `y=$#a`, `y=${#a[@]}`, `a+=(z)`} {
		t.Run(op, func(t *testing.T) {
			cost := func(size int) float64 {
				best := 0.0
				for i := range 3 {
					src := `typeset -F SECONDS; a=(); for ((i=0;i<` + strconv.Itoa(size) + `;i++)); do a+=(x); done
t0=$SECONDS; for ((i=0;i<500;i++)); do ` + op + `; done; print -r -- $(( SECONDS - t0 ))`
					out, st := runZsh(t, t.TempDir(), src)
					v, err := strconv.ParseFloat(strings.TrimSpace(out), 64)
					if st != 0 || err != nil {
						t.Fatalf("size %d: %q (status %d)", size, out, st)
					}
					if i == 0 || v < best {
						best = v
					}
				}
				return best
			}
			small, large := cost(2000), cost(20000)
			if small <= 0 {
				small = 1e-6
			}
			if ratio := large / small; ratio > 3 {
				t.Errorf("500 x %s took %.4fs on 2,000 elements and %.4fs on 20,000: ratio %.1f, want about 1",
					op, small, large, ratio)
			}
		})
	}
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
