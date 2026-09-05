// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
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
	if !s.IgnoredStaysInSession {
		t.Error("this shell keeps an ignored line in the list and only leaves it out of the file")
	}
	// The two rules bash keeps in HISTCONTROL are `setopt` names here, which
	// is a different startup surface. Naming a variable zsh does not have
	// would give this dialect a knob real zsh ignores.
	if s.Control != "" {
		t.Errorf("Control = %q, want nothing — zsh has no HISTCONTROL", s.Control)
	}
}
