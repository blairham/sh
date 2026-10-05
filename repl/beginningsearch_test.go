// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"io"
	"strings"
	"testing"
)

// The walk to entries that begin with the text before the cursor, and the move
// a line up or down. Every row is what zsh 5.9.2 was measured to do; the table
// is in beginningsearch.go.

var beginningHistory = []string{
	": echo apple", ": ls one", ": echo banana", ": echo apple", ": ls two", ": echo cherry",
}

// typedBeginning runs keys through an editor whose ^G and ^T are the backward
// and forward beginning search, with history to walk over. An `X` typed after
// the walk shows where it left the cursor.
func typedBeginning(t *testing.T, history []string, keys string) string {
	t.Helper()
	var out strings.Builder
	table := map[string]Binding{
		"\a":   {Widget: WidgetHistoryBeginningSearchBackward},
		"\x14": {Widget: WidgetHistoryBeginningSearchForward},
	}
	e := Shell{KeyBindings: func(Keymap) map[string]Binding { return table }}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = history
	e.browsing = len(history)
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

func TestTheBeginningSearchIsTheTextBeforeTheCursor(t *testing.T) {
	for _, tc := range []struct{ name, keys, want string }{
		{"the newest entry beginning with it, the cursor where it was", ": ec\aX\n", ": ecXho cherry"},
		{"then the next, past entries that do not begin with it", ": ec\a\aX\n", ": ecXho apple"},
		{"then older still", ": ec\a\a\aX\n", ": ecXho banana"},
		// The discriminating row against the matching walk: that one looks
		// for the first word, `:`, and would find `: echo cherry` here too —
		// but only the text before the cursor finds `: ls two` from `: l`.
		{"what is before the cursor, not the first word", ": l\aX\n", ": lXs two"},
		// And the cursor moved back: the text after it is not searched for.
		{"the text after the cursor is not part of it", ": lzz\x02\x02\aX\n", ": lXs two"},
		{"nothing beginning with it leaves the line alone", "zzz\aX\n", "zzzX"},
		// An empty prefix begins every entry.
		{"before the start of the line, the newest entry", "q\x01\aX\n", "X: echo cherry"},
		// Forward again comes back down through the matches to the line that
		// was being typed.
		{"forward to the newer match", ": ec\a\a\x14X\n", ": ecXho cherry"},
		{"forward past the newest brings the typed line back", ": ec\a\a\x14\x14X\n", ": ecX"},
		{"forward from the bottom finds nothing", ": ec\x14X\n", ": ecX"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := typedBeginning(t, beginningHistory, tc.keys); got != tc.want {
				t.Errorf("%q: got %q, want %q", tc.keys, got, tc.want)
			}
		})
	}
}

// An entry the same as the line is passed over: with `: x a` twice running,
// the second press would otherwise land on the line it was already showing.
func TestTheBeginningSearchPassesOverTheLineItIsShowing(t *testing.T) {
	history := []string{": x a", ": x b", ": x a", ": x a"}
	for _, tc := range []struct{ keys, want string }{
		{": x\a\n", ": x a"},
		{": x\a\a\n", ": x b"},
		{": x\a\a\a\n", ": x a"},
	} {
		if got := typedBeginning(t, history, tc.keys); got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.keys, got, tc.want)
		}
	}
}

// What a widget calling these by name is answered with: the line, the cursor
// and the status, through the same handle `zle NAME` reaches.
func TestTheBeginningSearchAndLineMovesAnswerAWidget(t *testing.T) {
	const three = "abcdef\nxy\nlonger line"
	for _, tc := range []struct {
		name       string
		w          Widget
		in         Line
		wantBuffer string
		wantCursor int
		wantStatus int
	}{
		{"search back", WidgetHistoryBeginningSearchBackward, Line{Buffer: ": ec", Cursor: 4}, ": echo cherry", 4, 0},
		{"search back, nothing", WidgetHistoryBeginningSearchBackward, Line{Buffer: "zzz", Cursor: 1}, "zzz", 1, 1},
		{"search forward from the bottom", WidgetHistoryBeginningSearchForward, Line{Buffer: "abc", Cursor: 1}, "abc", 1, 1},
		{"down a line, same column", WidgetDownLine, Line{Buffer: three, Cursor: 8}, three, 11, 0},
		{"up a line, the column clamped", WidgetUpLine, Line{Buffer: three, Cursor: 12}, three, 9, 0},
		{"up into an empty line", WidgetUpLine, Line{Buffer: "ab\n", Cursor: 3}, "ab\n", 0, 0},
		{"down into an empty line", WidgetDownLine, Line{Buffer: "ab\n", Cursor: 1}, "ab\n", 3, 0},
		// Not symmetric, as measured: up from the first line goes to the start.
		{"up from the first line", WidgetUpLine, Line{Buffer: three, Cursor: 5}, three, 0, 1},
		{"up from a single line", WidgetUpLine, Line{Buffer: "abc", Cursor: 1}, "abc", 0, 1},
		{"down from the last line", WidgetDownLine, Line{Buffer: three, Cursor: 15}, three, 15, 1},
		{"down from a single line", WidgetDownLine, Line{Buffer: "abc", Cursor: 1}, "abc", 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Shell{}.newEditor(t.Context(), nil)
			e.in, e.out = typing(""), io.Discard
			e.history = beginningHistory
			e.browsing = len(beginningHistory)
			e.drafts = map[int][]rune{}
			out, performed := editorActions{e: e, prompt: drawPrompt("$ ")}.Perform(tc.w, tc.in)
			if !performed {
				t.Fatal("refused")
			}
			if out.Buffer != tc.wantBuffer || out.Cursor != tc.wantCursor || out.Status != tc.wantStatus {
				t.Errorf("got %q @%d status %d, want %q @%d status %d",
					out.Buffer, out.Cursor, out.Status, tc.wantBuffer, tc.wantCursor, tc.wantStatus)
			}
		})
	}
}

// `zle copy-region-as-kill STRING`: the text is what the next yank brings
// back, and the line is left alone.
func TestAKillHandedInIsWhatTheNextYankBringsBack(t *testing.T) {
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing(""), io.Discard
	a := editorActions{e: e, prompt: drawPrompt("$ ")}
	a.Kill("foo bar")
	out, _ := a.Perform(WidgetYank, Line{Buffer: "xy", Cursor: 1})
	if out.Buffer != "xfoo bary" || out.Cursor != 8 {
		t.Errorf("got %q @%d, want %q @8", out.Buffer, out.Cursor, "xfoo bary")
	}
}
