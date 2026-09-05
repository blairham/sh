// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// Measured with the output drained after every keystroke — without that the
// terminal flushes what it has queued when the interrupt arrives, and what
// comes back is a screen that never existed. Here, real bash draws `^C` after the abandoned line.
func TestEditorStyle(t *testing.T) {
	const want = "^C"
	if got := bash.EditorStyle().Interrupt; got != want {
		t.Errorf("Interrupt = %q, want %q", got, want)
	}
}

// The question this shell asks before printing a large listing, and how it
// reads the answer. Measured under a pty.
func TestEditorStyleAsksBeforeALargeListing(t *testing.T) {
	s := bash.EditorStyle()
	if got, want := s.ListQuery, "Display all %[1]d possibilities? (y or n)"; got != want {
		t.Errorf("ListQuery = %q, want %q", got, want)
	}
	if !s.ListQueryAcceptsOnlyYesOrNo {
		t.Error("this shell rings the bell at anything that is not y or n and asks again")
	}
	if s.ListQueryEchoesTheKey {
		t.Error("this shell does not write the answering key back")
	}
}

// What this shell calls a word, and what its kills do with one.
//
// Measured under a pty against bash 5.3.15 started with neither startup files
// nor an inputrc, one keystroke at a time. Every one of these is a zero value,
// and every one of them is a measurement rather than an absence: zsh answers
// all five the other way.
func TestEditorStyleWords(t *testing.T) {
	s := bash.EditorStyle()
	// `M-b` on `echo /usr/local/bin` leaves the cursor in front of `bin`, so
	// a word here is letters and digits and nothing else.
	if got := s.WordCharacters; got != "" {
		t.Errorf("WordCharacters = %q, want nothing beyond letters and digits", got)
	}
	// `^U` with the cursor at the start of `echo one two` leaves the line as
	// it was, having nothing in front of the cursor to kill.
	if s.KillToStartOfLineTakesTheWholeLine {
		t.Error("^U kills what is before the cursor and leaves the rest")
	}
	// `^W` on `echo a+b` leaves `echo `, so it is delimited by whitespace and
	// not by the word its motion keys use — which `M-Delete` on the same line
	// is, leaving `echo a+`.
	if s.KillWordBeforeCursorUsesWordCharacters {
		t.Error("^W here is delimited by whitespace alone")
	}
	// `M-f` from the start of `echo one two` leaves the cursor after `echo`.
	if s.ForwardWordStopsBeforeTheNextWord {
		t.Error("M-f here stops at the end of the word, not before the next one")
	}
	// `^T` at the start of `echo abc` leaves the line alone.
	if s.TransposeAtTheStartSwapsTheFirstTwo {
		t.Error("^T here does nothing with no character in front of the cursor")
	}
}

// Taking a change back, and `M-.` past the oldest line it can reach.
//
// Measured under a pty against bash 5.3.15 and again against bash 3.2.57, one
// keystroke at a time, with the line read back out of the shell's own history
// file. The two versions agree on all three, and zsh answers all three the
// other way — so these zero values are measurements and not defaults nobody
// looked at.
func TestEditorStyleUndoAndLastArgument(t *testing.T) {
	s := bash.EditorStyle()
	// `echo abcdef` typed a character at a time and then one `^_` leaves an
	// empty line: the whole run of typing is one change. A run and not the
	// line — `echo abc`, `^B`, `d`, `^_` leaves `echo abc`.
	if s.UndoTakesBackOneKeystrokeAtATime {
		t.Error("^_ here takes back the whole run of typing")
	}
	// `echo one two`, `^A`, `^K`, `^_` leaves the cursor at the end of the
	// line, after what the undo put back rather than where the kill was.
	if s.UndoRestoresTheCursorToWhereItWas {
		t.Error("^_ here leaves the cursor after the text it put back")
	}
	// Three lines behind the prompt and four presses of `M-.`: the word this
	// shell had inserted comes off the line and nothing replaces it.
	if s.LastArgumentStaysOnTheOldestLine {
		t.Error("M-. here empties what it inserted once it runs out of lines")
	}
}
