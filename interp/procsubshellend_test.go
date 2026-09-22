// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// The floor the shell's own end of a substitution pipe is moved to is one
// above the highest number the dialect could publish.
//
// Derived from the wish list rather than named as a constant, because "out of
// the way" is a different number in each dialect — so this is the one part of
// the arrangement that can be checked without a pipe, a kernel or a table,
// and it is the part a reader has to trust for the rest to make sense.
func TestTheShellEndFloorClearsEveryNumberTheDialectCouldPublish(t *testing.T) {
	for _, tc := range []struct {
		name string
		want []int
		to   int
	}{
		{"bash, descending from the top of the table", []int{63, 62, 61}, 64},
		{"BusyBox, ascending above it", []int{64, 65, 66}, 67},
		{"ksh93, ascending from the first extra", []int{3, 4, 5}, 6},
		{"zsh, ascending from its own base", []int{11, 12, 13}, 14},
		{"an empty list has no floor", nil, 0},
		{"the highest wins wherever it sits in the order", []int{5, 90, 7}, 91},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shellEndFloor(tc.want); got != tc.to {
				t.Errorf("shellEndFloor(%v) = %d, want %d", tc.want, got, tc.to)
			}
		})
	}
}
