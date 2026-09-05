// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// Measured with the output drained after every keystroke — without that the
// terminal flushes what it has queued when the interrupt arrives, and what
// comes back is a screen that never existed. Here, real zsh draws no mark.
func TestEditorStyle(t *testing.T) {
	const want = ""
	if got := zsh.EditorStyle().Interrupt; got != want {
		t.Errorf("Interrupt = %q, want %q", got, want)
	}
}

// The question this shell asks before printing a large listing. It counts the
// rows as well as the matches, echoes the key, and takes the first key it is
// given as the answer.
func TestEditorStyleAsksBeforeALargeListing(t *testing.T) {
	s := zsh.EditorStyle()
	if got, want := s.ListQuery, "zsh: do you wish to see all %[1]d possibilities (%[2]d lines)? "; got != want {
		t.Errorf("ListQuery = %q, want %q", got, want)
	}
	if !s.ListQueryEchoesTheKey {
		t.Error("this shell writes the answering key back")
	}
	if s.ListQueryAcceptsOnlyYesOrNo {
		t.Error("this shell takes the first key, whatever it is")
	}
}

// What this shell calls a word, and what its kills do with one.
//
// Measured under a pty against zsh 5.9.2 started with no startup files, one
// keystroke at a time. These are the five places where the same key does
// something different from bash, and all five are on the daily path.
func TestEditorStyleWords(t *testing.T) {
	s := zsh.EditorStyle()
	// This shell's own WORDCHARS default, and checked a character at a time
	// by pressing the key: `M-b` on `echo /usr/local/bin` goes to the front
	// of the path and `M-b` on `echo a+b` does not, because `/` is in the
	// list and `+` is not.
	if got, want := s.WordCharacters, "*?_-.[]~=/&;!#$%^(){}<>"; got != want {
		t.Errorf("WordCharacters = %q, want %q", got, want)
	}
	// `^U` with the cursor at the start of `echo one two` empties the line.
	if !s.KillToStartOfLineTakesTheWholeLine {
		t.Error("^U here kills the whole line wherever the cursor is")
	}
	// `^W` on `echo a+b` leaves `echo a+`, stopping inside the argument.
	if !s.KillWordBeforeCursorUsesWordCharacters {
		t.Error("^W here uses the same word its motion keys use")
	}
	// `M-f` from the start of `echo one two` leaves the cursor before `one`.
	if !s.ForwardWordStopsBeforeTheNextWord {
		t.Error("M-f here stops before the next word, not at the end of this one")
	}
	// `^T` at the start of `echo abc` gives `ceho abc`.
	if !s.TransposeAtTheStartSwapsTheFirstTwo {
		t.Error("^T here swaps the first two characters and moves past them")
	}
}

// Taking a change back, and `M-.` past the oldest line it can reach.
//
// Measured under a pty against zsh 5.9.2, one keystroke at a time, with the
// line read back out of the shell's own history file. All three are
// disagreements with bash rather than absences.
func TestEditorStyleUndoAndLastArgument(t *testing.T) {
	s := zsh.EditorStyle()
	// `echo abcdef` typed a character at a time and then one `^_` leaves
	// `echo abcde` here; bash leaves an empty line.
	if !s.UndoTakesBackOneKeystrokeAtATime {
		t.Error("^_ here takes back one keystroke rather than the whole run of typing")
	}
	// `echo one two`, `^A`, `^K`, `^_` leaves the cursor at the start of the
	// line here — where it was when the kill happened — and at the end of it
	// in bash.
	if !s.UndoRestoresTheCursorToWhereItWas {
		t.Error("^_ here puts the cursor back where the change was made")
	}
	// Three lines behind the prompt and four presses of `M-.`: this shell
	// keeps the oldest line's last word and bash takes the word off the line.
	if !s.LastArgumentStaysOnTheOldestLine {
		t.Error("M-. here stops on the oldest line rather than emptying what it inserted")
	}
}
