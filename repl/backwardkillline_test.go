// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// backward-kill-line kills back to the start of the line and keeps what is
// ahead of the cursor, under the style whose `^U` takes the whole line too,
// and it counts lines. Measured 2026-10-05 against zsh 5.9.2 through a
// pseudo-terminal, from a widget that sets the line and cursor and runs
// `zle .backward-kill-line` with `-n` (#5960); ⏎ is a newline.
func TestBackwardKillLineKillsToTheLinesStart(t *testing.T) {
	const multi = "l1\nl2\nl3 x"
	unset := 1 << 30 // no count at all
	cases := []struct {
		buffer     string
		cursor, n  int
		want, cut  string
		wantCursor int
	}{
		{"aa bb cc dd ee", 7, unset, "c dd ee", "aa bb c", 0},
		{"aa bb cc dd ee", 0, unset, "aa bb cc dd ee", "", 0},
		{"aa bb cc dd ee", 14, unset, "", "aa bb cc dd ee", 0},
		{"aa bb cc dd ee", 7, 2, "c dd ee", "aa bb c", 0},
		{"aa bb cc dd ee", 7, -1, "aa bb c", "c dd ee", 7},
		{"aa bb cc dd ee", 7, -2, "aa bb c", "c dd ee", 7},
		{"aa bb cc dd ee", 7, 0, "aa bb cc dd ee", "", 7},
		{multi, 9, unset, "l1\nl2\nx", "l3 ", 6},
		{multi, 6, unset, "l1\nl2l3 x", "\n", 5},
		{multi, 6, 2, "l1\nl3 x", "l2\n", 3},
		{multi, 9, 2, "l1\nl2x", "\nl3 ", 5},
		{multi, 9, 3, "l1\nx", "l2\nl3 ", 3},
		{multi, 9, 9, "x", "l1\nl2\nl3 ", 0},
		{multi, 3, -1, "l1\n\nl3 x", "l2", 3},
		{multi, 3, -2, "l1\nl3 x", "l2\n", 3},
		{multi, 2, unset, "\nl2\nl3 x", "l1", 0},
		{multi, 0, -1, "\nl2\nl3 x", "l1", 0},
	}
	for _, c := range cases {
		var got Line
		var cut string
		typedReachingBackStyled(t, EditorStyle{KillToStartOfLineTakesTheWholeLine: true}, nil, nil, func(in Line, ed Actions) (Line, bool) {
			in.Buffer, in.Cursor, in.Numeric = c.buffer, c.cursor, nil
			if c.n != unset {
				n := c.n
				in.Numeric = &n
			}
			got, _ = ed.Perform(WidgetBackwardKillLine, in)
			cut = ed.CutBuffer()
			return Line{}, true
		}, "\a\n")
		if got.Buffer != c.want || got.Cursor != c.wantCursor || cut != c.cut {
			t.Errorf("%q at %d count %d: [%q] at %d cut %q, want [%q] at %d cut %q",
				c.buffer, c.cursor, c.n, got.Buffer, got.Cursor, cut, c.want, c.wantCursor, c.cut)
		}
	}
}

// Called from a widget, the count is the call's: with `$NUMERIC` unset the
// line is killed once, whatever count the keystroke that ran the widget
// spent. Measured: `ESC 3` and then a widget that unsets NUMERIC and runs
// `zle .backward-kill-line` on `l1⏎l2⏎l3 x` at 9 kills only `l3 `.
func TestBackwardKillLineFromAWidgetIsToldTheCallsCount(t *testing.T) {
	var got Line
	typedReachingBackStyled(t, EditorStyle{PrefixArgument: true}, nil, nil, func(in Line, ed Actions) (Line, bool) {
		if in.Numeric == nil {
			t.Fatal("the keystroke's count did not reach the widget, so this tests nothing")
		}
		in.Buffer, in.Cursor, in.Numeric = "l1\nl2\nl3 x", 9, nil
		got, _ = ed.Perform(WidgetBackwardKillLine, in)
		return Line{}, true
	}, "\x1b3\a\n")
	if got.Buffer != "l1\nl2\nx" {
		t.Errorf("line %q, want %q", got.Buffer, "l1\nl2\nx")
	}
}

// From a key, the count typed before it is the action's count and the key is
// not played again: measured against zsh 5.9.2 with `backward-kill-line` on
// `^U` and on `ESC u`, `ESC 2 ^U` and `ESC 2 ESC u` on `l1⏎l2⏎l3 x` with the
// cursor after `l3 ` both leave `l1⏎l2x`, and `ESC - ^U` on `aa bb cc dd ee`
// at 7 leaves `aa bb c`.
func TestBackwardKillLineFromAKeyTakesTheCount(t *testing.T) {
	for _, c := range []struct{ keys, want string }{
		{"aa bb cc dd ee\x01\x06\x06\x06\x06\x06\x06\x06\x15", "c dd ee"},
		{"aa bb cc dd ee\x01\x06\x06\x06\x06\x06\x06\x06\x1b-\x15", "aa bb c"},
		{"aa bb cc dd ee\x01\x06\x06\x06\x06\x06\x06\x06\x1b-\x1bu", "aa bb c"},
		{"\x0f\x1b2\x15", "l1\nl2x"},
		{"\x0f\x1b2\x1bu", "l1\nl2x"},
		{"\x0f\x15", "l1\nl2\nx"},
	} {
		var out strings.Builder
		s := Shell{
			Editor: EditorStyle{PrefixArgument: true, KillToStartOfLineTakesTheWholeLine: true},
			KeyBindings: func(Keymap) map[string]Binding {
				return map[string]Binding{
					"\x15":  {Widget: WidgetBackwardKillLine},
					"\x1bu": {Widget: WidgetBackwardKillLine},
					"\x0f":  {Function: "setup"},
				}
			},
		}
		e := s.newEditor(t.Context(), &terminalState{})
		e.runFunc = func(_ string, in Line, _ Actions) (Line, bool) {
			in.Buffer, in.Cursor = "l1\nl2\nl3 x", 9
			return in, true
		}
		e.in, e.out = typing(c.keys+"\r"), &out
		got, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%q: line %q, want %q", c.keys, got, c.want)
		}
	}
}
