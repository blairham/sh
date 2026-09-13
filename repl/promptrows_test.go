// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A prompt with a newline in it, which is two rows on the screen and two
// different jobs for the editor.
//
// The upper rows are written once, when the prompt is first drawn. Every
// redraw after that rewrites only the last row — the row the line is on, and
// the only one a keystroke can change. Writing the whole prompt each time is
// what this shell did, and a two-row prompt is where it showed: a fresh copy
// of the upper row went onto the screen at every keystroke, so pressing Up
// left a ladder of prompts behind it (#2467).

func TestAPromptIsSplitAtItsLastNewline(t *testing.T) {
	for _, c := range []struct {
		name, in, lead, text string
		cells                int
	}{
		{
			// The ordinary case, and the one this must not make cost
			// anything: no newline is all text and no lead.
			"one row is all text", "$ ", "", "$ ", 2,
		},
		{
			"two rows split at the newline",
			"~/some/dir main\n> ", "~/some/dir main\n", "> ", 2,
		},
		{
			// Several rows: everything before the *last* newline is lead,
			// because only the last row holds the line.
			"three rows keep two of them above",
			"one\ntwo\n$ ", "one\ntwo\n", "$ ", 2,
		},
		{
			// A prompt ending in a newline puts the line at the left margin.
			"a trailing newline leaves an empty last row",
			"only\n", "only\n", "", 0,
		},
		{
			"a bare newline", "\n", "\n", "", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			p := drawPrompt(c.in)
			if p.lead != c.lead || p.text != c.text {
				t.Errorf("split into lead %q text %q, want lead %q text %q",
					p.lead, p.text, c.lead, c.text)
			}
			if p.cells != c.cells {
				t.Errorf("cells = %d, want %d — the last row alone", p.cells, c.cells)
			}
		})
	}
}

// The width is the last row's, which is what every placement calculation
// needs: it is where the line starts on its row.
//
// Counting the upper rows put the wrap, the cursor column and the search's
// arithmetic out by the width of them — `place` takes this number and has no
// other way to know.
func TestAPromptsWidthIsItsLastRowOnly(t *testing.T) {
	p := drawPrompt("a very long upper row indeed\n> ")
	if p.cells != 2 {
		t.Errorf("cells = %d, want 2 — the upper row is not on the line's row", p.cells)
	}
}

// The markers work across the split: what they hide is out of the count and
// still on the screen, whichever row it is on.
func TestTheMarkersWorkOnEitherSideOfTheNewline(t *testing.T) {
	// A hidden escape on the upper row, and a visible `> ` below it.
	p := drawPrompt(markStart + "\x1b]0;title\a" + markEnd + "upper\n> ")
	if p.cells != 2 {
		t.Errorf("cells = %d, want 2", p.cells)
	}
	if !strings.Contains(p.lead, "\x1b]0;title\a") {
		t.Errorf("lead = %q, want the hidden text kept for the screen", p.lead)
	}
	if strings.ContainsAny(p.lead+p.text, markStart+markEnd) {
		t.Errorf("markers left in %q%q", p.lead, p.text)
	}
	// And hidden text on the *last* row is out of the count but still drawn.
	q := drawPrompt("upper\n" + markStart + "\x1b[32m" + markEnd + "> ")
	if q.cells != 2 {
		t.Errorf("cells = %d, want 2 — the color costs no columns", q.cells)
	}
	if !strings.Contains(q.text, "\x1b[32m") {
		t.Errorf("text = %q, want the color kept", q.text)
	}
}

// And the whole of it through the editor: the upper row is drawn once however
// many keystrokes redraw the line.
func TestATwoRowPromptDrawsItsUpperRowOnce(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("abc\x01x\n"), &out
	e.history = []string{"earlier"}
	e.browsing = 1
	if _, err := e.readLine(drawPrompt("UPPERROW\n> ")); err != nil {
		t.Fatal(err)
	}
	// Four characters typed and a ^A between them, so the line is redrawn
	// several times over. Before the split, each of those put another copy on
	// the screen.
	if n := strings.Count(out.String(), "UPPERROW"); n != 1 {
		t.Errorf("the upper row was drawn %d times, want once", n)
	}
	if !strings.Contains(out.String(), "> ") {
		t.Errorf("screen = %q, want the last row drawn", out.String())
	}
}
