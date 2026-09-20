// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The right prompt on a real terminal.
//
// The unit tests beside this one assert the placement arithmetic; they cannot
// tell whether a right prompt reaches the screen, because the editor is only
// built where there is a terminal. And the assertions here are over the
// **screen** — what a terminal of this width would be showing — rather than
// over the bytes, for the reason screenmodel_test.go gives: an incremental
// redraw writes whichever of several equivalent sequences is shortest, so a
// test naming one of them asserts a coincidence.
//
// A **two-row** prompt throughout, because a one-row prompt cannot catch a
// frame that is correct in its pieces and wrong in its nesting — and the right
// prompt belongs to the last row alone, which a one-row prompt cannot
// distinguish from the whole of it.

// rightWidth is chosen so the threshold is reachable in a few keystrokes:
// with an 80-column fixture and a four-cell last prompt row, cols - right - 2
// is 18, so 14 characters keep it and 15 do not.
const (
	rightWidth   = 60
	rightKeeps   = 14
	rightPromptR = "RRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRR"
)

func newRightPromptSession(t *testing.T) *session {
	t.Helper()
	drawn := 0
	return newSessionWith(t, func(sh *Shell) {
		sh.Theme = PromptThemeFunc(func(info PromptInfo) (ThemedPrompt, bool) {
			if info.Continued {
				return ThemedPrompt{Cont: "> "}, true
			}
			drawn++
			// The same `[N]` mark the fixture's own prompt carries, so the
			// harness can wait for a prompt that has not been drawn before —
			// and on the *second* row, so the right prompt has a last row to
			// belong to.
			return ThemedPrompt{
				Text:  "upper\n[" + itoa(drawn) + "] ",
				Right: rightPromptR,
			}, true
		})
	})
}

// It is drawn against the right-hand edge, and one cell short of it.
func TestARightPromptIsDrawnAgainstTheEdge(t *testing.T) {
	s := newRightPromptSession(t)
	s.typeLine("")

	row := s.row(1)
	at := strings.Index(row, rightPromptR)
	if at < 0 {
		t.Fatalf("no right prompt on the row the line is typed on:\n%q", row)
	}
	// 80 columns less 60 cells less the blank column at the edge.
	if want := fixtureCols - rightWidth - 1; at != want {
		t.Errorf("it starts at column %d, want %d:\n%q", at, want, row)
	}
	if end := len(strings.TrimRight(row, " ")); end != fixtureCols-1 {
		t.Errorf("the row is %d cells wide, want %d — the last column stays blank:\n%q",
			end, fixtureCols-1, row)
	}
	// And it is on the last row of the prompt, not the first.
	if strings.Contains(s.row(0), "R") {
		t.Errorf("the right prompt landed on a banner row:\n%q", s.row(0))
	}
	// A line accepted so the session has a fresh prompt to end at.
	s.typeKeys("\x15\n")
	s.end()
}

// It goes when the line grows into it and comes back when the line shrinks,
// at the width that was measured.
func TestARightPromptGoesWhenTheLineReachesItAndComesBack(t *testing.T) {
	s := newRightPromptSession(t)
	s.typeLine(strings.Repeat("x", rightKeeps))

	if !strings.Contains(s.row(1), rightPromptR) {
		t.Fatalf("it was dropped a column early:\n%q", s.row(1))
	}
	s.typeKeys("x")
	if strings.Contains(s.row(1), rightPromptR) {
		t.Errorf("it survived a line with no column between:\n%q", s.row(1))
	}
	// And the character that pushed it off is on the screen, so this is not
	// a shell that stopped drawing.
	if got := strings.TrimRight(s.row(1), " "); !strings.HasSuffix(got, strings.Repeat("x", rightKeeps+1)) {
		t.Errorf("the line is %q", got)
	}

	s.typeKeys("\x7f")
	if !strings.Contains(s.row(1), rightPromptR) {
		t.Errorf("it did not come back when the line shrank:\n%q", s.row(1))
	}

	s.typeKeys("\x15\n")
	s.end()
}

// An accepted line does not keep it. This is the one place we do not do what
// zsh does, and the reason is in rightprompt.go.
func TestAnAcceptedLineLeavesNoRightPromptBehind(t *testing.T) {
	s := newRightPromptSession(t)
	s.typeLine("echo hi\n")
	// Through the prompt after it, so the accepted row is finished and will
	// not be written to again.
	s.typeLine("\n")

	rows := strings.Split(s.shown().styledText(), "\n")
	for i, row := range rows {
		if !strings.Contains(row, "echo hi") {
			continue
		}
		if strings.Contains(row, "R") {
			t.Errorf("row %d kept its right prompt after the line was accepted:\n%q", i, row)
		}
		if !strings.Contains(row, "[1]") {
			t.Errorf("row %d is not the accepted line's own row:\n%q", i, row)
		}
	}
	if !strings.Contains(s.ran.String(), "hi") {
		t.Errorf("the command did not run; it printed %q", s.ran.String())
	}
	s.end()
}

// A transient prompt and a right prompt on the same accepted line, which is
// the case the two features create between them and neither owns.
//
// They meet in endLine and both act in front of toLastRow. The trim erases
// from the top of the prompt to the end of the screen, so where one is
// configured it takes the right prompt with it; where one is not, the erase
// beside it is what does.
//
// **What this test proves, and what it does not.** Deleting rightPromptErase
// outright leaves this test passing and fails
// TestAnAcceptedLineLeavesNoRightPromptBehind — checked, rather than reasoned
// about. So the erase is load-bearing only where no transient prompt is
// configured, and that other test is what holds it. This one holds the thing
// no single-feature test can see: that the two acting on the same row in the
// same breath leave a correct screen rather than a double erase, a cursor one
// column out, or a banner row half taken down.
func TestATransientPromptLeavesNoRightPromptEither(t *testing.T) {
	drawn := 0
	s := newSessionWith(t, func(sh *Shell) {
		sh.Transient = func() string { return "· " }
		sh.Theme = PromptThemeFunc(func(info PromptInfo) (ThemedPrompt, bool) {
			if info.Continued {
				return ThemedPrompt{Cont: "> "}, true
			}
			drawn++
			return ThemedPrompt{
				Text:  "upper\n[" + itoa(drawn) + "] ",
				Right: rightPromptR,
			}, true
		})
	})
	s.typeLine("echo hi\n")
	s.typeLine("\n")

	rows := strings.Split(s.shown().styledText(), "\n")
	found := false
	for i, row := range rows {
		if !strings.Contains(row, "echo hi") {
			continue
		}
		found = true
		if strings.Contains(row, "R") {
			t.Errorf("row %d kept its right prompt under a transient prompt:\n%q", i, row)
		}
		if !strings.HasPrefix(strings.TrimLeft(row, " "), "·") {
			t.Errorf("row %d was not trimmed to the transient prompt:\n%q", i, row)
		}
		// And the banner row above it is gone too, which is what the trim is
		// for — a right prompt erased off a frame that stayed would be half
		// the job.
		if i > 0 && strings.Contains(rows[i-1], "upper") {
			t.Errorf("the banner row above the accepted line survived:\n%q", rows[i-1])
		}
	}
	if !found {
		t.Fatalf("the accepted line is not on the screen:\n%q", s.screen.String())
	}
	s.end()
}
