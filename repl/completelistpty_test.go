// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// A grouped, described listing on a real terminal, under a two-row prompt.
//
// The unit tests in completelist_test.go grade the arrangement; this grades
// the bytes. It exists because a listing is the one part of completion that a
// reader-driven test cannot see the whole of: `list` writes through the
// editor's own `write`, and everything about where the rows land — the row the
// line was abandoned on, the returns between rows, the prompt drawn again
// underneath — is the pty's answer and not the function's.
//
// **Two rows of prompt, deliberately.** A one-row `PS1` cannot tell a listing
// that was drawn in the right place from one that ate the row above it, and
// this package has a standing finding that components can each be right while
// the nesting is broken (#2467, #3222). With `UPPER` above the prompt, a
// listing that overwrote it is a failed assertion rather than a screen that
// looks plausible.
func TestAGroupedListingReachesTheTerminal(t *testing.T) {
	described := Group{Name: "opts", Heading: "options", OnePerLine: true}
	bare := Group{Name: "levels"}
	s := newSessionWith(t, func(sh *Shell) {
		sh.Runner.Vars["PS1"] = "UPPER\n[\\#]"
		sh.KeyBindings = func(Keymap) map[string]Binding {
			return map[string]Binding{"\t": {Widget: WidgetComplete, Candidates: "w"}}
		}
		sh.RunCompletion = func(context.Context, string, Completion) []Candidate {
			// Interleaved, and each block's own two out of order, so that
			// what is drawn is the arrangement rather than the order they
			// were handed over in. A block is drawn where it was first
			// reached; `-f` is the first candidate, so the described block is
			// the first one on the screen.
			return []Candidate{
				{Word: "-f", Display: "-f  -- force overwrite", Group: described},
				{Word: "-2", Display: "-2", Group: bare},
				{Word: "-d", Display: "-d  -- decompress", Group: described},
				{Word: "-1", Display: "-1", Group: bare},
			}
		}
	})

	// Two presses: the first has nothing to fill in, since the four agree on
	// nothing past the `-`, and the second lists.
	s.typeLine(": -\t\t")
	waitFor(t, s.screen, "force overwrite", "the described row")

	// The rows the terminal is actually showing. The prompt is two rows, so
	// the listing starts at row 2 — and row 0 still says UPPER, which is the
	// half a one-row prompt could not have checked.
	if got := s.row(0); got != "UPPER" {
		t.Errorf("row 0 is %q, want the upper prompt row untouched", got)
	}
	want := []string{"options", "-d  -- decompress", "-f  -- force overwrite", "-1  -2"}
	for i, row := range want {
		if got := s.row(2 + i); got != row {
			t.Errorf("row %d is %q, want %q\nscreen:\n%s",
				2+i, got, row, s.shown().styledText())
		}
	}
	// And the prompt is drawn again under the listing with the line still on
	// it, because the line is still being typed. Its last row only: the rows
	// above a prompt are written once, which is the split drawnPrompt is
	// about and what keeps a two-row prompt from leaving a ladder behind it.
	if got := s.row(2 + len(want)); !strings.HasPrefix(got, "[1]: -") {
		t.Errorf("the line was not redrawn under the listing; row is %q", got)
	}

	// The line is run rather than abandoned, so the session reaches a second
	// prompt and ^D at it ends the session.
	s.typeKeys("\n")
	s.end()
}
