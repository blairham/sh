// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// `M-.`, the key that saves retyping the path that was just typed.
//
// Measured under a pty against bash 5.3.15, bash 3.2.57 and zsh 5.9.2, one
// keystroke at a time, with the resulting line read back out of the shell's
// own history file rather than off the screen. Everything here is what all
// three do, except the one case they part company on, which is a field.

// typedAfter runs a line through an editor that already has these lines behind
// it — which is what `M-.` reaches back into.
func typedAfter(t *testing.T, style EditorStyle, history []string, keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{Editor: style}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = history
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

func TestTheLastArgumentOfThePreviousLine(t *testing.T) {
	behind := []string{": a1 a2", ": b1 b2", ": c1 c2"}
	for _, c := range []struct {
		name    string
		history []string
		keys    string
		want    string
	}{
		{
			// The whole reason for the key: the argument is on the line
			// before, and typing it again is where the mistake comes from.
			"one press takes the line before",
			behind, ": X\x1b.\r", ": Xc2",
		},
		{
			// Pressed again straight away it walks a line further back, and
			// what the first press put in comes out — so the presses count
			// lines rather than piling words up.
			"a second press walks back a line",
			behind, ": X\x1b.\x1b.\r", ": Xb2",
		},
		{
			"a third goes back another",
			behind, ": X\x1b.\x1b.\x1b.\r", ": Xa2",
		},
		{
			// A keystroke in between ends the walk, and the press after it
			// starts again at the most recent line and inserts a second copy.
			"a key in between starts the walk again",
			behind, ": X\x1b.Q\x1b.\r", ": Xc2Qc2",
		},
		{
			// Both spellings are bound in both shells.
			"M-_ is the same key",
			behind, ": X\x1b_\r", ": Xc2",
		},
		{
			// Quotes hold a word together and come along with it; a
			// backslash does not, in either shell.
			"a quoted last word keeps its quotes",
			[]string{": p 'x y'"},
			": X\x1b.\r", ": X'x y'",
		},
		{
			"a backslash does not hold a word together",
			[]string{`: p a\ b`},
			": X\x1b.\r", ": Xb",
		},
		{
			"trailing whitespace is not a word",
			[]string{": a b   "},
			": X\x1b.\r", ": Xb",
		},
		{
			// The text goes in at the cursor and not at the end of the line.
			"it goes in at the cursor",
			[]string{": c1 c2"},
			": XY\x02\x1b.\r", ": Xc2Y",
		},
		{
			// A line with one word on it gives that word, which is the
			// command name.
			"a line of one word gives that word",
			[]string{": solo"},
			": X\x1b.\r", ": Xsolo",
		},
		{
			// Nothing behind the prompt at all: nothing is inserted, and a
			// second press does nothing either.
			"nothing behind the prompt",
			nil, ": X\x1b.\x1b.Z\r", ": XZ",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, style := range []EditorStyle{zeroAnswers, otherAnswers} {
				if got := typedAfter(t, style, c.history, c.keys); got != c.want {
					t.Errorf("%q gave %q, want %q", c.keys, got, c.want)
				}
			}
		})
	}
}

// What happens once `M-.` has been pressed more times than there are lines,
// which is the one thing the two shells do not agree about.
//
// Measured with three lines behind the prompt and four presses: bash takes the
// word it had inserted back off the line and puts nothing in its place, and
// zsh keeps the oldest line's last word. A fifth press leaves each of them
// where the fourth did — bash does not come back around to the most recent
// line, and zsh does not walk off the end.
func TestTheLastArgumentPastTheOldestLine(t *testing.T) {
	behind := []string{": a1 a2", ": b1 b2", ": c1 c2"}
	for _, c := range []struct {
		name    string
		style   EditorStyle
		presses int
		want    string
	}{
		{"the word comes off the line", zeroAnswers, 4, ": X"},
		{"and stays off", zeroAnswers, 5, ": X"},
		{"the oldest line is kept", otherAnswers, 4, ": Xa2"},
		{"and stays", otherAnswers, 5, ": Xa2"},
	} {
		t.Run(c.name, func(t *testing.T) {
			keys := ": X" + strings.Repeat("\x1b.", c.presses) + "\r"
			if got := typedAfter(t, c.style, behind, keys); got != c.want {
				t.Errorf("%d presses gave %q, want %q", c.presses, got, c.want)
			}
		})
	}
}

// The word the key inserts, on its own.
//
// Split on whitespace except inside quotes, and the quotes come with the word.
func TestTheLastWordOfALine(t *testing.T) {
	for _, c := range []struct{ line, want string }{
		{": a1 a2", "a2"},
		{": solo", "solo"},
		{"ls", "ls"},
		{"", ""},
		{"   ", ""},
		{": a b   ", "b"},
		{": p 'x y'", "'x y'"},
		{`: p "a b"`, `"a b"`},
		{`: p a\ b`, "b"},
		{": a | : b c", "c"},
		// A quote nobody closed runs to the end of the line, which is also
		// what a shell would make of it.
		{": p 'x y", "'x y"},
		// Quotes in the middle of a word are part of it.
		{`: pre"a b"post`, `pre"a b"post`},
	} {
		if got := lastWord(c.line); got != c.want {
			t.Errorf("lastWord(%q) = %q, want %q", c.line, got, c.want)
		}
	}
}
