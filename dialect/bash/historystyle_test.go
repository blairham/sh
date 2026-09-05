// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// Measured under a pseudo-terminal on 2026-09-05, bash 5.3.15, with the screen
// reconstructed from the bytes rather than read off the raw stream — `C-r` is
// drawn incrementally, so the stream is the optimisation and the screen is the
// behavior. With four lines of history and `C-r e c h o` typed:
//
//	(reverse-i-search)`echo': echo two
//
// and after two more `C-r`, once nothing older matches:
//
//	(failed reverse-i-search)`echo': echo one
func TestHistoryStyleDrawsTheSearchWhereThePromptWas(t *testing.T) {
	s := bash.HistoryStyle()
	if got, want := s.SearchPrompt, "(reverse-i-search)`%s': "; got != want {
		t.Errorf("SearchPrompt = %q, want %q", got, want)
	}
	if got, want := s.SearchFailedPrompt, "(failed reverse-i-search)`%s': "; got != want {
		t.Errorf("SearchFailedPrompt = %q, want %q", got, want)
	}
	if s.SearchBelowTheLine {
		t.Error("this shell replaces the prompt rather than drawing under the line")
	}
}

// The two variables, and the answer that makes them a conflict rather than a
// spelling difference.
//
// Measured: `HISTCONTROL=ignorespace` and a line typed with a leading space
// leaves the file without it *and* leaves the session without it — the up
// arrow at the next prompt recalls the line before, and bash's own `history`
// cannot see the hidden one. zsh, asked the same thing, keeps it.
func TestHistoryStyleForgetsWhatItIgnores(t *testing.T) {
	s := bash.HistoryStyle()
	if got, want := s.Control, "HISTCONTROL"; got != want {
		t.Errorf("Control = %q, want %q", got, want)
	}
	if got, want := s.Ignore, "HISTIGNORE"; got != want {
		t.Errorf("Ignore = %q, want %q", got, want)
	}
	if s.IgnoreIsOnePattern {
		t.Error("HISTIGNORE is a colon-separated list, not one pattern")
	}
	if s.IgnoredStaysInSession {
		t.Error("this shell drops an ignored line from the session as well as from the file")
	}
}
