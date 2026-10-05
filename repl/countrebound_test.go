// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strconv"
	"strings"
	"testing"
)

// typedRebound runs keys through an editor with a count and a table of
// rebound control keys, and reports the line and what the shell's widget on
// ^B and ^D was told.
func typedRebound(t *testing.T, table map[string]Binding, keys string) (line string, told []string) {
	t.Helper()
	var out strings.Builder
	s := Shell{
		Editor:      EditorStyle{PrefixArgument: true},
		KeyBindings: func(Keymap) map[string]Binding { return table },
	}
	e := s.newEditor(t.Context(), &terminalState{})
	e.runFunc = func(_ string, in Line, _ Actions) (Line, bool) {
		n := "unset"
		if in.Numeric != nil {
			n = strconv.Itoa(*in.Numeric)
		}
		told = append(told, n)
		return in, true
	}
	e.in, e.out = typing(keys+"\r"), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return got + "|" + strconv.Itoa(e.pos), told
}

// A count on a rebound control key is the binding's, measured 2026-10-05
// against zsh 5.9.2 through a pseudo-terminal (#6031): nothing turns the key
// into its byte's opposite, and an editor action a count turns the other way
// is turned by its own name.
func TestACountOnAReboundKeyIsTheBindings(t *testing.T) {
	table := map[string]Binding{
		"\x02": {Function: "n"},
		"\x06": {Widget: WidgetBackwardChar},
		"\x0b": {Widget: WidgetKillWordBefore},
		"\x17": {Widget: WidgetForwardWord},
	}
	for _, c := range []struct {
		keys, want, told string
	}{
		{"\x1b-\x02", "|0", "-1"},
		{"\x1b-\x1b2\x02", "|0", "-2"},
		{"\x1b3\x02", "|0", "3"},
		{"abcdef\x1b-\x06", "abcdef|6", ""},
		{"abcdef\x01\x1b-\x1b2\x06", "abcdef|2", ""},
		{"abcdef\x1b2\x06", "abcdef|4", ""},
		{"aa bb cc\x01\x1b-\x0b", " bb cc|0", ""},
		{"aa bb cc\x1b2\x0b", "aa |3", ""},
		// One kill for the count: the yank brings both words back.
		{"aa bb cc\x1b2\x0b\x19", "aa bb cc|8", ""},
		{"aa bb cc\x1b-\x17", "aa bb cc|6", ""},
	} {
		line, told := typedRebound(t, table, c.keys)
		if line != c.want || strings.Join(told, " ") != c.told {
			t.Errorf("%q: %s told %q, want %s told %q", c.keys, line, told, c.want, c.told)
		}
	}
}

// And a key that is not rebound, whose opposite is: `ESC -` and Backspace
// deletes forward and does not run what `^D` was rebound to.
func TestANegativeCountReachesTheOppositeActionNotItsKey(t *testing.T) {
	table := map[string]Binding{"\x04": {Function: "n"}, "\x02": {Function: "n"}}
	for _, c := range []struct{ keys, want string }{
		{"abcdef\x01\x06\x06\x1b-\x08", "abdef|2"},
		{"abcdef\x01\x06\x06\x1b-\x1b2\x7f", "abef|2"},
		{"abcdef\x1b-\x06", "abcdef|5"},
		{"abcdef\x1b2\x08", "abcd|4"},
	} {
		line, told := typedRebound(t, table, c.keys)
		if line != c.want || len(told) != 0 {
			t.Errorf("%q: %s told %q, want %s and no widget", c.keys, line, told, c.want)
		}
	}
}
