// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "testing"

// One edit, as the difference between two lines, and where each offset lands
// after it. The rows are the measurements in regionhighlight.go: zsh 5.9.2,
// `abcdefgh` in the line, offsets 1 3, 4 6 and 6 7.
func TestRegionOffsetsFollowOneEdit(t *testing.T) {
	offsets := []int{1, 3, 4, 6, 6, 7}
	for _, c := range []struct {
		name, was, now string
		cursor         int
		want           []int
	}{
		{"self-insert at 1", "abcdefgh", "ambcdefgh", 2, []int{2, 4, 5, 7, 7, 8}},
		{"self-insert at 2", "abcdefgh", "abbcdefgh", 3, []int{1, 4, 5, 7, 7, 8}},
		{"self-insert at 3", "abcdefgh", "abccdefgh", 4, []int{1, 4, 5, 7, 7, 8}},
		{"self-insert at 6", "abcdefgh", "abcdefdgh", 7, []int{1, 3, 4, 7, 7, 8}},
		{"self-insert at 8", "abcdefgh", "abcdefghe", 9, []int{1, 3, 4, 6, 6, 7}},
		{"delete-char at 2", "abcdefgh", "abdefgh", 2, []int{1, 2, 3, 5, 5, 6}},
		{"backward-kill-word from 5", "abcdefgh", "fgh", 0, []int{0, 0, 0, 1, 1, 2}},
		{"kill-line from 3", "abcdefgh", "abc", 3, []int{1, 3, 3, 3, 3, 3}},
		{"yank XY at 2", "abcdefgh", "abXYcdefgh", 4, []int{1, 5, 6, 8, 8, 9}},
		// A run of equal characters: the difference alone puts an insert of
		// `b` after `ab` at 2, and the cursor says it went in at 1.
		{"self-insert b at 1", "abcdefgh", "abbcdefgh", 2, []int{2, 4, 5, 7, 7, 8}},
		// And with no cursor to ask, the far end of the run.
		{"no cursor", "abcdefgh", "abbcdefgh", -1, []int{1, 4, 5, 7, 7, 8}},
	} {
		at, removed, inserted := lineEdit([]rune(c.was), []rune(c.now), c.cursor)
		for i, p := range offsets {
			if got := shiftRegionOffset(p, at, removed, inserted); got != c.want[i] {
				t.Errorf("%s: offset %d went to %d, want %d", c.name, p, got, c.want[i])
			}
		}
	}
}
