// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strconv"
	"strings"
	"testing"
)

// A count typed before a key, against the line and cursor zsh 5.9.2 left,
// measured 2026-10-02 through a pseudo-terminal (#5498). See prefixarg.go.
func TestACountIsSpentByTheNextKey(t *testing.T) {
	for _, c := range []struct {
		keys, line string
	}{
		{"abcdef\x1b3\x02X", "abcXdef"},
		{"echo \x1b4z", "echo zzzz"},
		{"\x1b1\x1b2z", strings.Repeat("z", 12)},
		{"\x1b12z", "2z"},
		{"abc\x1b0z", "abc"},
		{"\x1b-\x1b2\x02X", "X"},
		{"abcdef\x1b2\x08", "abcd"},
		{"abcdef\x01\x1b2\x04", "cdef"},
		{"abcdef\x1b-\x1b2\x08", "abcdef"},
		{"abcdef\x02\x02\x1b-\x1b2\x08", "abcd"},
		{"abcdef\x01\x06\x06\x06\x1b-\x1b2\x04", "adef"},
		{"abcdef\x01\x1b-\x1b2\x02X", "abXcdef"},
		{"abcdef\x1b-\x1b2\x06X", "abcdXef"},
		{"ab cd ef\x1b2\x1bbX", "ab Xcd ef"},
		{"ab cd ef\x01\x1b-\x1b2\x1bbX", "ab cd Xef"},
		{"ab cd ef\x1b2\x17", "ab "},
		{"ab\x1b3\x14", "ba"},
		{"abc\x1b2\x1b[DX", "aXbc"},
		{"abc\x1b-z", "abcz"},
		{"abc\x1b5", "abc"},
	} {
		var out strings.Builder
		// zsh's word move, which stops in front of the next word.
		style := EditorStyle{PrefixArgument: true, ForwardWordStopsBeforeTheNextWord: true}
		e := Shell{Editor: style}.newEditor(t.Context(), &terminalState{})
		e.in, e.out = typing(c.keys+"\r"), &out
		got, err := e.readLine(drawPrompt("$ "))
		if err != nil {
			t.Fatal(err)
		}
		if got != c.line {
			t.Errorf("%q: line %q, want %q", c.keys, got, c.line)
		}
	}
}

// A negative count types the character and leaves the cursor in front of
// it — `abc ESC - z` is `abcz` with the cursor at 3 — and a widget of the
// shell's is called once and told the count.
func TestACountReachesAWidgetOnceAndANegativeTypeStaysBehind(t *testing.T) {
	var out strings.Builder
	s := Shell{
		Editor:      EditorStyle{PrefixArgument: true},
		KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
	}
	e := s.newEditor(t.Context(), &terminalState{})
	var calls []string
	e.runFunc = func(_ string, in Line, _ Actions) (Line, bool) {
		n := "unset"
		if in.Numeric != nil {
			n = strconv.Itoa(*in.Numeric)
		}
		calls = append(calls, n+"@"+itoa(in.Cursor))
		return in, true
	}
	e.in, e.out = typing("abc\x1b-z\a\x1b3\x1b4\a\a\x1b-\a\r"), &out
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(calls, " "), "unset@3 34@3 unset@3 -1@3"; got != want {
		t.Errorf("widget calls %q, want %q", got, want)
	}
}

// A key bound to a widget of the shell's spends a count once, whether its
// first byte is ESC or a key a count would otherwise play again — zsh calls
// the widget once and tells it the count.
func TestABoundWidgetSpendsACountOnce(t *testing.T) {
	var out strings.Builder
	s := Shell{
		Editor: EditorStyle{PrefixArgument: true},
		KeyBindings: func(Keymap) map[string]Binding {
			return map[string]Binding{"\x02": {Function: "w"}, "\x1bq": {Function: "w"}}
		},
	}
	e := s.newEditor(t.Context(), &terminalState{})
	var calls []string
	e.runFunc = func(_ string, in Line, _ Actions) (Line, bool) {
		n := "unset"
		if in.Numeric != nil {
			n = strconv.Itoa(*in.Numeric)
		}
		calls = append(calls, n)
		return in, true
	}
	e.in, e.out = typing("\x1b3\x02\x1b2\x1bqz\r"), &out
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if want := "3 2"; strings.Join(calls, " ") != want {
		t.Errorf("widget calls %q, want %q", strings.Join(calls, " "), want)
	}
	if got != "z" {
		t.Errorf("line %q, want %q: the count must not outlive the key that spent it", got, "z")
	}
}
