// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// lastWordHistory is the history every row below was measured with, against
// zsh 5.9.2 through a pseudo-terminal on 2026-10-05, emacs keymap (#5987).
var lastWordHistory = []string{": one two three", ": alpha beta gamma"}

// lastWordStyle is zsh's: a count, the prefix argument that types one, and
// the walk that stays on the oldest line.
var lastWordStyle = EditorStyle{
	PrefixArgument: true, InsertLastWordTakesArguments: true, LastArgumentStaysOnTheOldestLine: true,
}

// lastWordEditor is an editor in that style with that history, and with `^X`
// a widget that runs each call in calls in turn, one per press.
func lastWordEditor(t *testing.T, keys string, calls [][]string) string {
	t.Helper()
	var out strings.Builder
	s := Shell{
		Editor:      lastWordStyle,
		KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\x18": {Function: "w"}} },
	}
	e := s.newEditor(t.Context(), &terminalState{})
	e.in, e.out = typing(keys+"\r"), &out
	e.history = append([]string(nil), lastWordHistory...)
	e.browsing = len(e.history)
	press := 0
	e.runFunc = func(_ string, in Line, ed Actions) (Line, bool) {
		args := calls[press]
		press++
		if args == nil {
			got, _ := ed.Perform(WidgetInsertLastWord, in)
			return got, true
		}
		got, _ := ed.(ArgumentActions).PerformWith(WidgetInsertLastWord, in, args)
		return got, true
	}
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return got
}

// TestInsertLastWordTakesACount: from a key, the count picks the word — a
// positive one from the end and nought or a negative one from the start —
// and the walk goes on through a count typed between two presses.
func TestInsertLastWordTakesACount(t *testing.T) {
	for _, c := range []struct{ keys, want string }{
		{"\x1b.\x1b.", "three"},
		{"\x1b2\x1b.", "beta"},
		{"\x1b2\x1b.\x1b.", "three"},
		{"\x1b.\x1b2\x1b.", "two"},
		{"\x1b.\x1b-\x1b.", "one"},
		{"\x1b0\x1b.", ":"},
		{"\x1b-\x1b.", "alpha"},
		{"\x1b-\x1b1\x1b.", "alpha"},
		{"\x1b-\x1b1\x1b.\x1b.", "three"},
		{"\x1b5\x1b.", ""},
		{"\x1b.\x1b5\x1b.", "gamma"},
		{"\x1b.\x1b5\x1b.\x1b.", "gamma"},
		{"\x1b.\x1b.\x1b.\x1b.", "three"},
		{"ab\x1b.", "abgamma"},
	} {
		if got := lastWordEditor(t, c.keys, nil); got != c.want {
			t.Errorf("%q: line %q, want %q", c.keys, got, c.want)
		}
	}
}

// TestInsertLastWordTakesArguments: from a widget, the offset, the word,
// and whether the offset counts from the line being edited.
func TestInsertLastWordTakesArguments(t *testing.T) {
	a := func(args ...string) []string { return args }
	for _, c := range []struct {
		keys  string
		calls [][]string
		want  string
	}{
		{"\x18", [][]string{a("-1", "-2")}, "beta"},
		{"\x18\x18", [][]string{a("-1", "-2"), a("-1", "-2")}, "two"},
		{"\x1b.\x18", [][]string{a("0", "-3")}, "alpha"},
		{"\x18", [][]string{a("0", "-1")}, ""},
		{"\x1b.\x18", [][]string{a("1", "-1")}, ""},
		{"\x18", [][]string{a("1", "-1")}, ""},
		{"\x1b.\x1b.\x18", [][]string{a("-1", "-1", "1")}, "gamma"},
		{"\x1b.\x1b.\x1b.\x18", [][]string{a("1", "-1")}, "gamma"},
		{"\x18", [][]string{a("-1", "0")}, "gamma"},
		{"\x18", [][]string{a("x", "y")}, ""},
		{"\x1b2\x18", [][]string{a("-1")}, "beta"},
		{"\x1b2\x18", [][]string{a("-1", "-1")}, "gamma"},
		{"\x1b.\x18\x18", [][]string{a("-1", "5"), a("0", "-3")}, "one"},
		{"\x1b.\x18\x18", [][]string{a("-1", "5"), a("-1", "-1")}, "gamma"},
		{"k \x1b.\x18\x18", [][]string{a("-1", "5"), a("0", "-1")}, "k three"},
		{"k \x1b.\x18\x18", [][]string{a("-1", "5"), a("0", "-2", "1")}, "k "},
		{"x \x18\x18", [][]string{nil, nil}, "x three"},
		{"\x18 z\x18", [][]string{nil, nil}, "gamma zgamma"},
		// The line being edited, without what the last call put in it.
		{"p q r \x18\x18", [][]string{a("0", "-1"), a("0", "-2")}, "p q r q"},
		{"p q r \x18\x18\x18\x18", [][]string{a("0", "-1"), a("0", "-2"), a("0", "-3"), a("0", "-4")}, "p q r "},
	} {
		if got := lastWordEditor(t, c.keys, c.calls); got != c.want {
			t.Errorf("%q %v: line %q, want %q", c.keys, c.calls, got, c.want)
		}
	}
}

// TestInsertLastWordStatus: a call that finds no word, or no line, or is
// given words that are not numbers, answers 1 and leaves the line.
func TestInsertLastWordStatus(t *testing.T) {
	for _, c := range []struct {
		args []string
		want int
	}{
		{[]string{"-1", "5"}, 1},
		{[]string{"1", "-1"}, 1},
		{[]string{"x", "y"}, 1},
		{[]string{"-1", "-1"}, 0},
		{[]string{"-1", "-1", "1", "1"}, 0},
	} {
		var status int
		typedReachingBackStyled(t, lastWordStyle, nil, lastWordHistory, func(in Line, ed Actions) (Line, bool) {
			out, _ := ed.(ArgumentActions).PerformWith(WidgetInsertLastWord, in, c.args)
			status = out.Status
			return Line{}, true
		}, "\a\n")
		if status != c.want {
			t.Errorf("%v: status %d, want %d", c.args, status, c.want)
		}
	}
}

// TestHistNoNumbersTheLineBeingEdited: `$HISTNO` is 3 at a fresh prompt after
// two lines and 2 and 1 after Up once and twice, measured.
func TestHistNoNumbersTheLineBeingEdited(t *testing.T) {
	e := &editor{history: lastWordHistory, historyCount: func() int { return 2 }}
	for browsing, want := range map[int]int{2: 3, 1: 2, 0: 1} {
		e.browsing = browsing
		if got := e.histNo(); got != want {
			t.Errorf("browsing %d: HISTNO %d, want %d", browsing, got, want)
		}
	}
}
