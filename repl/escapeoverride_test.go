// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// A key nobody bound does what it does with no bindings at all, whatever else
// somebody bound with the same first byte.
//
// That is what the override layer promises, and for escape sequences it did
// not hold until #5865. The binding lookup reads a key while anything bound
// could still be it and gave up at the first byte nothing continued to,
// dropping what it had read — which for a control sequence is the middle of
// it, so the rest was typed into the line. Any binding beginning with ESC was
// enough, and macOS's `/etc/zshrc` makes three by `$terminfo[kcuu1]` and its
// fellows, so every zsh session on that machine had the bug: a paste's opening
// marker gave up at `\e[2` and typed `00~` and then the paste, newline and all.
//
// Two tables, each holding one sequence nobody presses here: the
// application-mode arrow that `/etc/zshrc` binds, and a `\e[` one, which
// reaches the give-up by the other prefix. Every row is compared against the
// same keys with no table, which is the editor's own dispatch — escape.go —
// reading the sequence whole.
func TestAKeyNobodyBoundIsTheSameKeyWhateverElseIsBound(t *testing.T) {
	tables := map[string]map[string]Binding{
		"an application-mode arrow": {"\x1bOA": {Widget: WidgetPreviousHistoryMatching}},
		"a control sequence":        {"\x1b[99Z": {Widget: WidgetKillWholeLine}},
	}
	for _, row := range []struct {
		name, seq string
		// want is the accepted line for "ab cd" + seq + "Z\n", so that the
		// rows are measurements and not only agreements: both readings
		// agreeing on something wrong cannot pass.
		want string
	}{
		{"a paste", "\x1b[200~X\nY\x1b[201~", "ab cdX\nYZ"},
		{"F5", "\x1b[15~", "ab cdZ"},
		{"F12", "\x1b[24~", "ab cdZ"},
		{"Insert", "\x1b[2~", "ab cdZ"},
		{"Ctrl-Left", "\x1b[1;5D", "ab Zcd"},
		{"Home in application mode", "\x1bOH", "Zab cd"},
		{"a stray closing marker", "\x1b[201~", "ab cdZ"},
		{"an old-style mouse click", "\x1b[M !!", "ab cdZ"},
	} {
		t.Run(row.name, func(t *testing.T) {
			keys := "ab cd" + row.seq + "Z\n"
			plain := typedBound(t, nil, keys)
			if plain != row.want {
				t.Fatalf("with nothing bound %q gave %q, want %q — the row is wrong, not the lookup",
					keys, plain, row.want)
			}
			for name, table := range tables {
				if got := typedBound(t, table, keys); got != plain {
					t.Errorf("with %s bound, %q gave %q; with nothing bound it gives %q",
						name, keys, got, plain)
				}
			}
		})
	}
}

// The binding the table above stands in for still runs, so the rows are not
// passing because the table was never read.
func TestTheStandInEscapeBindingRuns(t *testing.T) {
	table := map[string]Binding{"\x1b[99Z": {Widget: WidgetKillWholeLine}}
	if got := typedBound(t, table, "ab cd\x1b[99ZZ\n"); got != "Z" {
		t.Errorf("the bound sequence gave %q, want %q", got, "Z")
	}
}

// And `$KEYS` is the sequence once, not the part the lookup read and then the
// whole of it again: the give-up hands the bytes back to be read a second
// time, and the record of the keystroke has to give them back too.
func TestAGivenBackSequenceIsRecordedOnce(t *testing.T) {
	e := Shell{KeyBindings: func(Keymap) map[string]Binding {
		return map[string]Binding{"\x1b[99Z": {Widget: WidgetKillWholeLine}}
	}}.newEditor(t.Context(), nil)
	e.keyBytes = []byte{esc}
	e.in = typing("[15~")
	if _, claimed, _ := e.matchBinding(esc); claimed {
		t.Fatal("the lookup claimed a sequence nothing is bound to")
	}
	if string(e.keyBytes) != "\x1b" {
		t.Errorf("after the give-up the keystroke records %q, want just the ESC", e.keyBytes)
	}
	// What was read past the ESC, and only that: the lookup gives up at
	// `\e[15`, where `\e[1~` stops being a possibility, and the `~` is still
	// in the input for the dispatch to read after these.
	if string(e.pushed) != "[15" {
		t.Errorf("handed back %q, want %q", e.pushed, "[15")
	}
}
