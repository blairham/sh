// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// WalkTo moves the walk to a line by its number, and a number no line has
// moves nothing. The rows are #6050's, measured against zsh 5.9.2 after `:
// one` and `: two` with `ab` typed.
func TestWalkToMovesTheWalkByNumber(t *testing.T) {
	history := []string{": one", ": two"}
	for _, c := range []struct {
		to     []int
		want   string
		cursor int
		histNo int
	}{
		{[]int{1}, ": one", 5, 1},
		{[]int{2}, ": two", 5, 2},
		{[]int{9}, "ab", 1, 3},
		{[]int{0}, "ab", 1, 3},
		{[]int{1, 3}, "ab", 2, 3},
	} {
		var got Line
		typedReachingBackStyled(t, EditorStyle{}, func(e *editor) {
			e.historyCount = func() int { return 2 }
		}, history, func(in Line, ed Actions) (Line, bool) {
			in.Buffer, in.Cursor = "ab", 1
			for _, n := range c.to {
				in = ed.(HistoryActions).WalkTo(n, in)
			}
			got = in
			return Line{Buffer: "x"}, true
		}, "\a\n")
		if got.Buffer != c.want || got.Cursor != c.cursor || got.HistNo != c.histNo {
			t.Errorf("WalkTo %v: %q at %d, HISTNO %d; want %q at %d, %d",
				c.to, got.Buffer, got.Cursor, got.HistNo, c.want, c.cursor, c.histNo)
		}
	}
}
