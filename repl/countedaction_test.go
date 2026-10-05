// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// An action performed from outside the editor is given the count the line
// carries — zsh's `$NUMERIC` — and plays it the way zsh 5.9.2 does, measured
// 2026-10-04 through a pseudo-terminal from a widget on `aa bb cc dd ee`
// with the cursor at 7 (#5941): twice for 2, the other way for -2, not at all
// for 0, and an end of the line is gone to once whatever the count. The cursor and
// line are zsh's on every row but the forward word moves; see there.
func TestAnActionFromOutsideTakesTheLinesCount(t *testing.T) {
	cases := []struct {
		w      Widget
		n      int
		buffer string
		cursor int
	}{
		{WidgetForwardChar, 2, "aa bb cc dd ee", 9},
		{WidgetForwardChar, -2, "aa bb cc dd ee", 5},
		{WidgetForwardChar, 0, "aa bb cc dd ee", 7},
		{WidgetBackwardChar, 2, "aa bb cc dd ee", 5},
		// This editor's default style stops a forward word move at the end
		// of a word, where zsh's stops at the start of the next one — zsh
		// lands at 12 here. That is the style's question, not the count's;
		// the count is the two moves.
		{WidgetForwardWord, 2, "aa bb cc dd ee", 11},
		{WidgetBackwardWord, 2, "aa bb cc dd ee", 3},
		{WidgetBackwardWord, -2, "aa bb cc dd ee", 11},
		{WidgetKillWordAfter, 2, "aa bb c ee", 7},
		{WidgetKillWordAfter, -2, "aa c dd ee", 3},
		{WidgetKillWordBefore, 2, "aa c dd ee", 3},
		{WidgetDeleteChar, 2, "aa bb cdd ee", 7},
		{WidgetDeleteChar, -2, "aa bbc dd ee", 5},
		{WidgetBackwardDeleteChar, 2, "aa bbc dd ee", 5},
		{WidgetBeginningOfLine, 2, "aa bb cc dd ee", 0},
		{WidgetBeginningOfLine, -2, "aa bb cc dd ee", 14},
		{WidgetEndOfLine, 0, "aa bb cc dd ee", 7},
		{WidgetEndOfLine, -1, "aa bb cc dd ee", 0},
	}
	for _, c := range cases {
		var got Line
		typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
			in.Buffer, in.Cursor, in.Numeric = "aa bb cc dd ee", 7, &c.n
			got, _ = ed.Perform(c.w, in)
			return Line{}, true
		}, "\a\n")
		if got.Buffer != c.buffer || got.Cursor != c.cursor {
			t.Errorf("widget %d count %d: [%s] at %d, want [%s] at %d", c.w, c.n, got.Buffer, got.Cursor, c.buffer, c.cursor)
		}
	}
}

// The kills a count makes are one kill, measured: `NUMERIC=2; zle kill-word`
// from the start of `aa bb cc dd ee` leaves `aa bb` in `$CUTBUFFER`.
func TestACountedKillIsOneKill(t *testing.T) {
	var cut string
	typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		n := 2
		in.Buffer, in.Cursor, in.Numeric = "aa bb cc dd ee", 0, &n
		ed.Perform(WidgetKillWordAfter, in)
		cut = ed.CutBuffer()
		return Line{}, true
	}, "\a\n")
	if cut != "aa bb" {
		t.Errorf("the kill holds %q, want %q", cut, "aa bb")
	}
}

// With no count an action is performed once, which is what every caller
// before #5941 was given and must still be.
func TestAnActionWithNoCountIsPerformedOnce(t *testing.T) {
	var got Line
	typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		in.Buffer, in.Cursor = "aa bb cc dd ee", 7
		got, _ = ed.Perform(WidgetForwardWord, in)
		return Line{}, true
	}, "\a\n")
	if got.Cursor != 8 {
		t.Errorf("cursor %d, want 8", got.Cursor)
	}
}
