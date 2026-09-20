// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The threshold is the one measured against zsh 5.9.2 through a pty, and it
// is a formula rather than a number — so the rows here are the six
// configurations that were measured, not a restatement of the arithmetic.
//
// docs/spec/prompt-theme.md carries the run. The widest line that keeps the
// right prompt is cols - right - 2: one blank column at the edge, and one
// between the line and the prompt.
func TestTheRightPromptIsKeptWhileAColumnStandsBetween(t *testing.T) {
	for _, c := range []struct {
		cols, right, widest int
	}{
		{40, 5, 33},
		{40, 1, 37},
		{40, 10, 28},
		{30, 5, 23},
		{80, 5, 73},
		{20, 3, 15},
	} {
		// Split as a three-cell prompt and the rest typed, which is how it
		// was measured — the formula is over the two together.
		const promptCells = 3
		if got := c.cols - c.right - 2; got != c.widest {
			t.Fatalf("the measurement and the formula disagree at %d/%d: %d", c.cols, c.right, got)
		}
		if !rightFits(promptCells, c.widest-promptCells, c.right, c.cols) {
			t.Errorf("at %d columns a %d-cell right prompt was dropped beside a line of %d",
				c.cols, c.right, c.widest)
		}
		if rightFits(promptCells, c.widest-promptCells+1, c.right, c.cols) {
			t.Errorf("at %d columns a %d-cell right prompt survived a line of %d",
				c.cols, c.right, c.widest+1)
		}
	}
}

// A terminal too narrow for both sides is that same rule and not a special
// case — measured at 7 columns, where the threshold is zero and a three-cell
// prompt already exceeds it.
func TestATerminalTooNarrowForBothSidesIsTheSameRule(t *testing.T) {
	if rightFits(3, 0, 5, 7) {
		t.Error("a 5-cell right prompt was drawn in 7 columns beside a 3-cell prompt")
	}
	if rightFits(3, 0, 5, 10) != true {
		t.Error("10 columns is exactly enough and was refused")
	}
}

// No right half, and no width to place one in, are both nothing drawn. A
// width of zero is not a width of eighty.
func TestNothingIsDrawnWithoutARightHalfOrAWidth(t *testing.T) {
	if rightFits(3, 0, 0, 80) {
		t.Error("an empty right prompt was placed")
	}
	if rightFits(3, 0, 5, 0) {
		t.Error("a right prompt was placed in a terminal of unknown width")
	}
}

// It sits against the edge and one cell short of it, which is where zsh puts
// it — 40 columns, a five-cell right prompt, columns 35 through 39 of 40, or
// 34 through 38 counted from zero.
func TestTheRightPromptSitsWhereZshPutsIt(t *testing.T) {
	if got := rightPromptAt(5, 40); got != 34 {
		t.Errorf("a 5-cell right prompt starts at column %d of 40, want 34", got)
	}
	if got := rightPromptAt(5, 40) + 5; got != 39 {
		t.Errorf("it ends at column %d, want 39 — leaving the last column blank", got)
	}

	// And the bytes: from a three-cell prompt with nothing typed, that is a
	// forward move of 31 — which is the number zsh wrote.
	var b strings.Builder
	prompt := drawnPrompt{cells: 3, right: "RIGHT", rightCells: 5}
	if !writeRightPrompt(&b, prompt, 0, 40) {
		t.Fatal("it was not drawn")
	}
	if got, want := b.String(), "\x1b[31CRIGHT\r"; got != want {
		t.Errorf("drawn as %q, want %q", got, want)
	}
}

// It moves with the edge and not with the line: ten characters typed put the
// cursor ten columns further along, and the forward move is ten shorter.
func TestItStaysAgainstTheEdgeWhileTheLineGrows(t *testing.T) {
	prompt := drawnPrompt{cells: 3, right: "RIGHT", rightCells: 5}
	for _, c := range []struct{ typed, forward int }{{0, 31}, {10, 21}, {30, 1}} {
		var b strings.Builder
		if !writeRightPrompt(&b, prompt, c.typed, 40) {
			t.Fatalf("it was not drawn beside a line of %d", c.typed)
		}
		want := "\x1b[" + itoa(c.forward) + "CRIGHT\r"
		if got := b.String(); got != want {
			t.Errorf("beside a line of %d it is %q, want %q", c.typed, got, want)
		}
	}
	// One more and there is no column between them, so nothing is written at
	// all — which is what takes it off the screen, because the whole-line
	// draw has already erased to the end of the screen.
	var b strings.Builder
	if writeRightPrompt(&b, prompt, 31, 40) {
		t.Errorf("it was drawn with no column between: %q", b.String())
	}
	if b.Len() != 0 {
		t.Errorf("something was written for a right prompt that does not fit: %q", b.String())
	}
}

// Accepting a line takes it off the row, which is the one thing zsh does not
// do. The erase runs from the end of the line so the gap goes with it, and the
// cursor is put back where it was found.
func TestAnAcceptedLineDoesNotKeepItsRightPrompt(t *testing.T) {
	prompt := drawnPrompt{cells: 3, right: "RIGHT", rightCells: 5}

	var b strings.Builder
	rightPromptErase(&b, prompt, 10, 40, 8)
	if got, want := b.String(), "\r\x1b[13C\x1b[K\r\x1b[8C"; got != want {
		t.Errorf("the erase is %q, want %q", got, want)
	}

	// Nothing to erase when there was nothing drawn, and nothing written for
	// it either: an erase on a row with no right prompt on it would clear
	// whatever a wrapped line had put there.
	var none strings.Builder
	rightPromptErase(&none, prompt, 31, 40, 8)
	if none.Len() != 0 {
		t.Errorf("a row with no right prompt on it was erased: %q", none.String())
	}
}
