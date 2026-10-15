// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strconv"
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
		`y=${a[5]}`, `y=${#a[5]}`, `y=$(( a[5] ))`, `(( y = a[5] + 1 ))`)
}
