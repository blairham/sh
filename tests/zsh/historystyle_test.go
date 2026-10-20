// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// Measured under a pseudo-terminal on 2026-09-05, zsh 5.9.2, beside the same
// probe run against bash — and the two differ in placement as well as in
// wording. zsh leaves the prompt and the line where they are and draws the
// search on a row of its own below:
//
//	P> echo two
//	bck-i-search: echo_
//
// with `failing bck-i-search: echo_` once nothing older matches. The trailing
// underscore is zsh's own and is part of the wording rather than a cursor.
func TestHistoryStyleDrawsTheSearchBelowTheLine(t *testing.T) {
	s := zsh.HistoryStyle()
	if got, want := s.SearchPrompt, "bck-i-search: %s_"; got != want {
		t.Errorf("SearchPrompt = %q, want %q", got, want)
	}
	if got, want := s.SearchFailedPrompt, "failing bck-i-search: %s_"; got != want {
		t.Errorf("SearchFailedPrompt = %q, want %q", got, want)
	}
	if !s.SearchBelowTheLine {
		t.Error("this shell keeps the prompt and draws the search underneath")
	}
}

// One pattern rather than a list, and an ignored line that is still there.
//
// Measured: with `HISTORY_IGNORE='ls*'`, `ls -d .` runs, is absent from the
// file afterwards, and is exactly what the up arrow recalls at the next
// prompt. bash asked the same thing forgets it entirely, and that conflict on
// identical intent is why this is an axis.
func TestHistoryStyleKeepsWhatItDoesNotWrite(t *testing.T) {
	s := zsh.HistoryStyle()
	if got, want := s.Ignore, "HISTORY_IGNORE"; got != want {
		t.Errorf("Ignore = %q, want %q", got, want)
	}
	if !s.IgnoreIsOnePattern {
		t.Error("HISTORY_IGNORE is a single pattern; a colon in it is not a separator")
	}
	if !s.PatternIgnoredStaysInSession {
		t.Error("this shell keeps a HISTORY_IGNORE line in the list and only leaves it out of the file")
	}
	// The two rules bash keeps in HISTCONTROL are option names here, which is
	// a different namespace. Naming a variable zsh does not have would give
	// this dialect a knob real zsh ignores.
	if s.Control != "" {
		t.Errorf("Control = %q, want nothing — zsh has no HISTCONTROL", s.Control)
	}
	if got, want := s.IgnoreSpaceOption, "HIST_IGNORE_SPACE"; got != want {
		t.Errorf("IgnoreSpaceOption = %q, want %q", got, want)
	}
	if got, want := s.IgnoreDupsOption, "HIST_IGNORE_DUPS"; got != want {
		t.Errorf("IgnoreDupsOption = %q, want %q", got, want)
	}
}

// The axis is the pattern knob's alone, and this dialect is the one shell that
// proves it, because it gives both answers.
//
// Measured 2026-09-06, zsh 5.9.2, `-f -i` on a pipe with a scratch HOME,
// ZDOTDIR and HISTFILE, `fc -l` read inside the session and the file read
// after `fc -W`:
//
//	HISTORY_IGNORE='echo hidden'   `fc -l` entry 5 is `echo hidden`; the file has
//	                               `echo one` then `echo two` and not it
//	setopt HIST_IGNORE_SPACE       `fc -l` goes `echo one`, `echo two`; the space-led
//	                               line is in neither
//	setopt HIST_IGNORE_DUPS        `fc -l` goes `echo a`, `echo b`, `echo a`; the
//	                               repeat is in neither
//
// So the two option-spelled rules answer the way bash 5.3.15, bash 3.2.57 and
// bash-as-sh answer all three of theirs, and the pattern knob is the only
// place the panel parts company. docs/spec/history.md carried the "kept"
// reading against HIST_IGNORE_SPACE until this was re-measured; it is the
// reading of HISTORY_IGNORE, taken in the same session.
func TestTheOptionSpelledRulesTakeNoAxis(t *testing.T) {
	s := zsh.HistoryStyle()
	// There is deliberately no per-option field to assert the absence of.
	// What pins the behavior is repl's own rules, which return "not
	// recallable" for these two whatever a dialect says about patterns —
	// TestWhichIgnoredLinesStayRecallable there. This asserts the half that
	// lives in the table: the names exist and the axis is spelled for
	// patterns only, so a later edit that renamed it back to a dialect-wide
	// answer would have to change this line to compile.
	if s.IgnoreSpaceOption == "" || s.IgnoreDupsOption == "" {
		t.Fatal("the two option-spelled rules are not named")
	}
	if !s.PatternIgnoredStaysInSession {
		t.Error("the axis is the pattern knob's and this dialect keeps such a line")
	}
}

// The two history options moved out of "recorded" when the front end learned
// to read them, and this is the join that says so: `setopt` writes the state
// and the same namespace a session reads answers with it.
//
// The session half is repl's — its rules read `interp.Runner.DialectOption`,
// which is the namespace this dialect installs. What can be asked of a script
// is that the state moves, that it reads back under every spelling zsh folds
// together, and that the two names are the ones HistoryStyle points at. A test
// that only asked `setopt` would pass for an option nothing could read.
func TestTheHistoryOptionsAreReadableThroughTheNamespace(t *testing.T) {
	style := zsh.HistoryStyle()
	for _, tc := range []struct {
		named string
		on    string
		off   string
	}{
		{named: style.IgnoreSpaceOption, on: "setopt hist_ignore_space", off: "unsetopt hist_ignore_space"},
		{named: style.IgnoreDupsOption, on: "setopt hist_ignore_dups", off: "unsetopt hist_ignore_dups"},
	} {
		if tc.named == "" {
			t.Fatal("HistoryStyle names no option here; the rule would be off in every session")
		}
		// Every spelling zsh folds into one, asked through the condition —
		// which is the same lookup DialectOption performs.
		for _, spelling := range []string{tc.named, strings.ToLower(tc.named), strings.ReplaceAll(strings.ToLower(tc.named), "_", "")} {
			src := tc.on + `; [[ -o ` + spelling + ` ]] && echo on; ` +
				tc.off + `; [[ -o ` + spelling + ` ]] || echo off`
			out, st := runZsh(t, t.TempDir(), src)
			if want := "on\noff\n"; out != want || st != 0 {
				t.Errorf("%s: out %q status %d, want %q at 0", spelling, out, st, want)
			}
		}
	}
}
