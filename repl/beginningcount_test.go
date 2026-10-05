// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// countedHistory is the history the rows below were measured with, against
// zsh 5.9.2 through a pseudo-terminal on 2026-10-05 (#5988).
var countedHistory = []string{": echo apple", ": ls x", ": echo banana"}

// typedBeginningCounted is typedBeginning with a count to type, and with ^X a
// widget that performs the backward search with the count n.
//
// Forward is on ^N, the key it was measured on, and not on ^T as in
// typedBeginning: a negative count on ^T is turned positive before the key's
// binding is looked up, which is #6031 and not this.
func typedBeginningCounted(t *testing.T, keys string, n *int) string {
	t.Helper()
	var out strings.Builder
	table := map[string]Binding{
		"\a":    {Widget: WidgetHistoryBeginningSearchBackward},
		"\x0e":  {Widget: WidgetHistoryBeginningSearchForward},
		"\x1bp": {Widget: WidgetHistoryBeginningSearchBackward},
		"\x18":  {Function: "w"},
	}
	e := Shell{
		Editor:      EditorStyle{PrefixArgument: true},
		KeyBindings: func(Keymap) map[string]Binding { return table },
	}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = countedHistory
	e.browsing = len(countedHistory)
	e.runFunc = func(_ string, in Line, ed Actions) (Line, bool) {
		in.Numeric = n
		got, _ := ed.Perform(WidgetHistoryBeginningSearchBackward, in)
		return got, true
	}
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

func TestTheBeginningSearchGoesToTheCountsMatch(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{": ec\a\n", ": echo banana"},
		{": ec\x1b2\a\n", ": echo apple"},
		{": ec\x1b3\a\n", ": ec"},
		{": ec\x1b0\a\n", ": echo banana"},
		{": ec\x1b-\a\n", ": ec"},
		{": ec\x1b-\x1b2\a\n", ": ec"},
		{": ec\a\a\x1b-\x0e\n", ": echo apple"},
		{": ec\a\a\x1b2\x0e\n", ": ec"},
		{": ec\a\a\x1b-\x1b2\a\n", ": ec"},
		{": ec\a\a\x1b3\x0e\n", ": echo apple"},
		{"zz\x1b2\a\n", "zz"},
		// A key that begins with ESC is performed once with the count, not
		// played again as many times.
		{": ec\x1b2\x1bp\n", ": echo apple"},
	} {
		if got := typedBeginningCounted(t, tc.keys, nil); got != tc.want {
			t.Errorf("%q: line %q, want %q", tc.keys, got, tc.want)
		}
	}
}

// From a widget the count is `$NUMERIC`, and the status says whether the
// match was there: measured, `M-2` then `zle .history-beginning-search-backward`
// is `: echo apple` at 0, and with 3 or 5 the line stays at 1.
func TestTheBeginningSearchFromAWidgetTakesNumeric(t *testing.T) {
	for _, tc := range []struct {
		n      int
		want   string
		status int
	}{
		{2, ": echo apple", 0},
		{3, ": ec", 1},
		{5, ": ec", 1},
	} {
		var out strings.Builder
		var status int
		n := tc.n
		table := map[string]Binding{"\x18": {Function: "w"}}
		e := Shell{KeyBindings: func(Keymap) map[string]Binding { return table }}.newEditor(t.Context(), nil)
		e.in, e.out = typing(": ec\x18\n"), &out
		e.history = countedHistory
		e.browsing = len(countedHistory)
		e.runFunc = func(_ string, in Line, ed Actions) (Line, bool) {
			in.Numeric = &n
			got, _ := ed.Perform(WidgetHistoryBeginningSearchBackward, in)
			status = got.Status
			return got, true
		}
		line, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatal(err)
		}
		if line != tc.want || status != tc.status {
			t.Errorf("count %d: line %q at %d, want %q at %d", tc.n, line, status, tc.want, tc.status)
		}
	}
}
