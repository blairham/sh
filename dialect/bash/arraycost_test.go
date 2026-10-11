// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strconv"
	"strings"
	"testing"
)

// Reading one element inside an expression costs the same on a large array as
// on a small one (#6358), and so do the other one-element reads beside it.
//
// `s=$(( s + a[i] ))` over an array read the whole array into a list for every
// element it took, so the loop was quadratic: 2.5 s for 40,000 elements where
// bash 5.3.20 takes 0.19. The same read written `${a[i]}` was already O(1)
// (#6117), which is what said the cost was the expression's route to the
// element and not the store.
//
// The shape of the measurement is dialect/zsh's TestArrayOperationsDoNotGrowWithTheArray:
// the same 500 operations on 2,000 elements and on 20,000, timed by the shell's
// own clock around the loop only, the best of three. An operation that walks
// the array costs ten times as much on the larger one.
func TestArrayReadsDoNotGrowWithTheArray(t *testing.T) {
	if testing.Short() {
		t.Skip("times loops over large arrays")
	}
	for _, op := range []string{`y=${a[5]}`, `y=${#a[5]}`, `y=$(( a[5] ))`, `(( y = a[5] + 1 ))`} {
		t.Run(op, func(t *testing.T) {
			cost := func(size int) float64 {
				best := 0.0
				for i := range 3 {
					src := `a=(); for ((i=0;i<` + strconv.Itoa(size) + `;i++)); do a+=(7); done
t0=${EPOCHREALTIME/./}; for ((i=0;i<500;i++)); do ` + op + `; done; echo $(( ${EPOCHREALTIME/./} - t0 ))`
					out, st := runBash(t, t.TempDir(), src)
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
				small = 1
			}
			if ratio := large / small; ratio > 3 {
				t.Errorf("500 x %s took %.0fus on 2,000 elements and %.0fus on 20,000: ratio %.1f, want about 1",
					op, small, large, ratio)
			}
		})
	}
}
