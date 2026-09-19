// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// What a completion key writes when nothing reaches the line, on a real
// terminal.
//
// The unit tests grade what `complete` decides; these grade the bytes, and
// the bytes are the whole of this — a completion that computes the right
// matches and draws none of them is indistinguishable at the keyboard from
// one that computes nothing. That was the state this package was in: a word
// whose matches agree on nothing past what is typed answered Tab with **zero
// bytes**, no bell, no listing and no redraw (#3714).
//
// **Two rows of prompt, deliberately**, for the reason
// completelistpty_test.go gives: with `UPPER` above the prompt, a listing
// that ate the row above it is a failed assertion rather than a screen that
// looks plausible.
//
// The option name is this test's own. This package names no shell: what it
// holds is that *some* option decides whether the matches are drawn on the
// keystroke that found them ambiguous, and which name that is belongs to a
// dialect.
const listMatchesOption = "listmatches"

// bellSession is a session whose Tab has two matches agreeing on nothing past
// the word, so that no completion can reach the line.
func bellSession(t *testing.T, listsOnTheSameKey bool) *session {
	t.Helper()
	return newSessionWith(t, func(s *Shell) {
		s.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		// A directory of its own and an empty one, so that the shell's own
		// file completion cannot answer a word this test means to go
		// unanswered. See completers: the first answer wins.
		s.Runner.Dir = t.TempDir()
		s.Completers = []Completer{CompleterFunc(func(c Completion) []Candidate {
			switch c.Word {
			case "a":
				// Two words agreeing on nothing past the `a` that was typed,
				// so there is no prefix to fill in and the keystroke changes
				// nothing.
				return Words("abbey", "azure")
			case "p":
				// Two agreeing on `pre`, so the keystroke puts two more
				// characters on the line and the word is still not settled.
				return Words("present", "pretend")
			case "s":
				// One, so the keystroke settles the word.
				return Words("solitary")
			}
			return nil
		})}
		s.Editor.ListMatchesWithoutASecondKeyOption = listMatchesOption
		s.Runner.SetOptionNamespace(func(_ *interp.Runner, name string) (bool, bool) {
			if name != listMatchesOption {
				return false, false
			}
			return listsOnTheSameKey, true
		})
	})
}

// TestAnAmbiguousCompletionRingsTheBellWhereTheMatchesWaitForASecondKey is the
// axis at rest: the keystroke says so and draws nothing else.
func TestAnAmbiguousCompletionRingsTheBellWhereTheMatchesWaitForASecondKey(t *testing.T) {
	se := bellSession(t, false)
	se.typeLine(": a")
	// The Tab, and then an ordinary key: the editor reads keystrokes in
	// order, so a marker drawn by the key *after* the Tab is proof that
	// everything the Tab was going to draw has been drawn. An assertion that
	// something is absent needs that; a bare wait on the bell does not give
	// it.
	se.typeKeys("\tZ")
	waitFor(t, se.screen, "\aZ", "the key typed after the Tab")

	drawn := se.screen.String()
	if n := strings.Count(drawn, "\a"); n != 1 {
		t.Errorf("the bell rang %d time(s), want exactly one\nscreen: %q", n, drawn)
	}
	if strings.Contains(drawn, "abbey") {
		t.Errorf("the matches were listed on the first key, where this axis waits for a second\nscreen: %q", drawn)
	}
	se.typeKeys("\n")
	se.end()
}

// TestASecondCompletionKeyListsTheMatchesWithoutRingingAgain is the other half
// of that axis, and the row that says the bell belongs to the completion
// rather than to the listing.
func TestASecondCompletionKeyListsTheMatchesWithoutRingingAgain(t *testing.T) {
	se := bellSession(t, false)
	se.typeLine(": a")
	se.typeKeys("\t\t")
	waitFor(t, se.screen, "abbey", "the listing on the second key")

	drawn := se.screen.String()
	if n := strings.Count(drawn, "\a"); n != 1 {
		t.Errorf("the bell rang %d time(s), want one — the key that lists is silent\nscreen: %q", n, drawn)
	}
	if got := se.row(0); got != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", got)
	}
	se.typeKeys("\n")
	se.end()
}

// TestAnAmbiguousCompletionListsOnTheSameKeyWhereTheAxisSaysSo is the axis
// moved, and it is the row #3714 was filed about.
func TestAnAmbiguousCompletionListsOnTheSameKeyWhereTheAxisSaysSo(t *testing.T) {
	se := bellSession(t, true)
	se.typeLine(": a")
	se.typeKeys("\t")
	waitFor(t, se.screen, "abbey", "the listing on the first key")

	drawn := se.screen.String()
	if n := strings.Count(drawn, "\a"); n != 1 {
		t.Errorf("the bell rang %d time(s), want one beside the listing\nscreen: %q", n, drawn)
	}
	// The bell comes first and the listing after it, which is the order the
	// reference draws them in.
	if i, j := strings.Index(drawn, "\a"), strings.Index(drawn, "abbey"); i > j {
		t.Errorf("the listing was drawn before the bell\nscreen: %q", drawn)
	}
	if got := se.row(0); got != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", got)
	}
	se.typeKeys("\n")
	se.end()
}

// TestACompletionThatMatchesNothingWritesTheBellAndNothingElse is the byte
// assertion the bell exists for.
//
// A bell moves no cursor and prints nothing, so a redraw beside it would be
// bytes nothing else on a terminal sends — and this editor sends none. Written
// as the exact delta rather than as a count, because a count passes for an
// editor that rings and then repaints the row.
func TestACompletionThatMatchesNothingWritesTheBellAndNothingElse(t *testing.T) {
	se := bellSession(t, false)
	// `q` is a word this session's completer declines and the empty scratch
	// directory cannot answer either, so nothing at all matches.
	se.typeLine(": q")
	before := se.screen.String()
	se.typeKeys("\tZ")
	waitFor(t, se.screen, "\aZ", "the bell and the key typed after the Tab")

	if got := se.screen.String()[len(before):]; !strings.HasPrefix(got, "\aZ") {
		t.Errorf("the Tab and the key after it wrote %q, want the bell and then the key", got)
	}
	se.typeKeys("\n")
	se.end()
}

// TestAnAmbiguousCompletionThatFillsAPrefixInIsSilentAtRest is the core's
// reading, and the row the other axis is measured against.
func TestAnAmbiguousCompletionThatFillsAPrefixInIsSilentAtRest(t *testing.T) {
	se := bellSession(t, false)
	se.typeLine(": p")
	se.typeKeys("\tZ")
	waitFor(t, se.screen, "preZ", "the filled-in prefix and the key after it")

	if drawn := se.screen.String(); strings.Contains(drawn, "\a") {
		t.Errorf("the bell rang for a keystroke that filled a prefix in\nscreen: %q", drawn)
	}
	se.typeKeys("\n")
	se.end()
}

// TestAnAmbiguousCompletionThatFillsAPrefixInRingsWhereTheAxisSaysSo is the
// axis moved, and the bell comes **before** the text it fills in.
func TestAnAmbiguousCompletionThatFillsAPrefixInRingsWhereTheAxisSaysSo(t *testing.T) {
	se := newSessionWith(t, func(s *Shell) {
		s.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		s.Runner.Dir = t.TempDir()
		s.Completers = []Completer{CompleterFunc(func(c Completion) []Candidate {
			switch c.Word {
			case "p":
				return Words("present", "pretend")
			case "s":
				return Words("solitary")
			}
			return nil
		})}
		s.Editor.BellRingsOnAnAmbiguousCompletionThatInserts = true
	})
	se.typeLine(": p")
	se.typeKeys("\tZ")
	waitFor(t, se.screen, "\areZ", "the bell, the prefix and the key after it")

	// The control, on the same session and the same axis: a keystroke that
	// **settles** the word is silent even here, which is what says this axis
	// is about the middle state rather than about ringing more often.
	se.typeKeys("\n")
	se.typeLine(": s")
	se.typeKeys("\t")
	waitFor(t, se.screen, "solitary ", "the settled word")
	if n := strings.Count(se.screen.String(), "\a"); n != 1 {
		t.Errorf("the bell rang %d time(s) over both lines, want the one that did not settle the word\nscreen: %q",
			n, se.screen.String())
	}
	se.typeKeys("\n")
	se.end()
}
