// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The history walk restricted to what the line already says.
//
// Nothing here names a shell. What the two keys are *called* is asserted in
// dialect/zsh; every row below is what the behavior was measured to be, and
// widgets.go carries the table it came from.

// typedSearching runs a line through an editor whose ^G and ^T walk history
// the matching way, with entries to walk over.
func typedSearching(t *testing.T, history []string, keys string) string {
	t.Helper()
	var out strings.Builder
	table := map[string]Binding{
		"\a":   {Widget: WidgetPreviousHistoryMatching},
		"\x14": {Widget: WidgetNextHistoryMatching},
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

var searchHistory = []string{"echo one", "ls -la", "echo two", "print hello"}

// The whole of it, row by row, against what zsh 5.9.2 was measured to do
// through a pseudo-terminal on 2026-09-12 with the same four entries.
func TestTheMatchingWalkIsTheLinesFirstWord(t *testing.T) {
	for _, tc := range []struct{ name, keys, want string }{
		{
			// The plain case: walk to entries beginning with what is typed.
			"the newest match, then the next",
			"echo\a\a\n", "echo one",
		},
		{
			// **The discriminating row.** Nothing begins with `echo zz`, and
			// it matches anyway — so what is searched for is the first word
			// and not the whole line.
			"the rest of the line is not part of the search",
			"echo zz\a\n", "echo two",
		},
		{
			// And not the text before the cursor either: ^A puts the cursor
			// at the start, where that reading would search for nothing.
			"the cursor is not the question",
			"echo\x01\a\n", "echo two",
		},
		{
			"a shorter word matches more",
			"ec\a\n", "echo two",
		},
		{
			"nothing matching leaves the line alone",
			"zzz\a\n", "zzz",
		},
		{
			// An empty line is the other half of these keys: the ordinary
			// walk, which is what a person meets first.
			"an empty line walks plainly",
			"\a\n", "print hello",
		},
		{
			// A line that is not empty and whose first word is. Both have an
			// empty first word and they are different questions: this one
			// finds nothing, where the empty line above walks.
			"a line beginning with a blank finds nothing",
			" echo\a\n", " echo",
		},
		{
			"a lone blank is the same",
			" \a\n", " ",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := typedSearching(t, searchHistory, tc.keys); got != tc.want {
				t.Errorf("line = %q, want %q", got, tc.want)
			}
		})
	}
}

// A run of these keys is **one walk**, and the search is not recomputed from
// the line on every press.
//
// This is the row that makes the difference on an empty line, which is the
// ordinary way a person meets these keys. The first press walks plainly; by
// the second the line holds a recalled entry with a first word of its own, and
// recomputing would turn the plain walk into a search for that word and stop
// dead. Measured: empty line then Up, Up, Up gives `print hello`, `echo two`,
// `ls -la`, where recomputing gives `print hello` three times.
func TestARunOfMatchingWalksIsOneWalk(t *testing.T) {
	if got, want := typedSearching(t, searchHistory, "\a\a\a\n"), "ls -la"; got != want {
		t.Errorf("line = %q, want %q — the plain walk must carry on", got, want)
	}
	// And a keystroke that is not one of them starts a new walk: after typing
	// a character the line reads `print hellox`, whose first word matches
	// nothing older, so the next press finds nothing.
	if got, want := typedSearching(t, searchHistory, "\ax\a\n"), "print hellox"; got != want {
		t.Errorf("line = %q, want %q — typing must end the walk", got, want)
	}
}

// Walking back down to the bottom brings back what was being typed, which is
// browse's own bookkeeping and is why this hands off to it rather than moving
// the line itself.
func TestTheMatchingWalkComesBackToTheTypedLine(t *testing.T) {
	if got, want := typedSearching(t, searchHistory, "echo\a\a\x14\x14\n"), "echo"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// Running off the end leaves the line on the last match rather than clearing
// it or wrapping.
func TestTheMatchingWalkStopsAtTheOldestMatch(t *testing.T) {
	if got, want := typedSearching(t, searchHistory, "echo\a\a\a\a\n"), "echo one"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}
