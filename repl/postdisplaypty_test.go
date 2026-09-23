// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// Text drawn after the line without being part of it: what an inline
// suggestion is made of. #4217.
//
// The assertions are over the **screen** — what a terminal of this width would
// be showing, styles and all — and not over the bytes, for the reason
// screenmodel_test.go gives twice over: a redraw writes whichever equivalent
// sequence is shortest, and a test that strips the styling before looking
// throws away the half this feature is about. A suggestion that is drawn in no
// color at all is not a suggestion; it is the line with somebody else's text
// glued to it.
//
// A **two-row** prompt throughout, for the reason the right-prompt fixtures
// give: a one-row prompt cannot tell a frame that is correct in its pieces from
// one that is wrong in its nesting, and where a postdisplay goes is a question
// about the last row.
//
// Measured 2026-09-22 through a pseudo-terminal against zsh 5.9.2, a widget
// bound to a key: with `abcdef` typed and a widget setting `POSTDISPLAY='<PD>'`
// and `CURSOR=2`, that shell draws `abcdef<PD>` and moves the cursor 8 cells
// left — the rest of the line *and* the whole postdisplay — so the text goes
// after the line's end and the cursor stays inside the line. Accepting runs
// `abZcdef`: the postdisplay is not part of the command.
const suggestion = " --sugg"

// newSuggestingSession is a session whose `w` action sets a postdisplay and
// asks for it to be colored, which is what zsh-autosuggestions does with
// `POSTDISPLAY` and `region_highlight` between them.
func newSuggestingSession(t *testing.T, style string) *session {
	t.Helper()
	drawn := 0
	return newSessionWith(t, func(sh *Shell) {
		sh.Theme = PromptThemeFunc(func(info PromptInfo) (ThemedPrompt, bool) {
			if info.Continued {
				return ThemedPrompt{Cont: "> "}, true
			}
			// Two rows, with the mark the fixture waits for on the second —
			// counted here, the way the right-prompt fixture counts it.
			drawn++
			return ThemedPrompt{Text: "upper\n[" + itoa(drawn) + "] "}, true
		})
		sh.KeyBindings = func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Function: "w"}}
		}
		sh.RunWidget = func(_ context.Context, _ string, in Line) (Line, bool) {
			in.Postdisplay = suggestion
			return in, true
		}
		// What colors it: a run reaching past the line, which is how that
		// shell's `region_highlight` names a postdisplay — the offsets count
		// one text and not two.
		sh.Highlighter = HighlighterFunc(func(line string) []Highlight {
			at := strings.Index(line, suggestion)
			if at < 0 {
				return nil
			}
			return []Highlight{{Start: at, End: at + len(suggestion), Style: style}}
		})
	})
}

// The suggestion is on the screen, after the line, in the color it was asked
// for — and the cursor is where the line ends, not where the suggestion does.
func TestAPostdisplayIsDrawnAfterTheLine(t *testing.T) {
	const style = "\x1b[38;5;8m"
	s := newSuggestingSession(t, style)
	s.typeLine("echo hi")
	waitForLine(t, s.screen, "[1] echo hi", "the line typed")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] echo hi"+suggestion, "the suggestion drawn")

	shown := s.shown()
	// The style, cell for cell. A screen model that kept only the characters
	// would pass the wait above over a suggestion drawn in the line's own
	// color, which is the blind spot this repository has been bitten by.
	if want := style + suggestion + highlightReset; !strings.Contains(shown.styledText(), want) {
		t.Errorf("the suggestion is not drawn in the style asked for.\nscreen:\n%s", shown.styledText())
	}
	// And the cursor: after `hi` and before the suggestion, which is the whole
	// of what makes it a suggestion rather than text somebody typed.
	row, col := shown.at()
	if want := len("[1] echo hi"); col != want {
		t.Errorf("the cursor is at row %d column %d, want column %d — after the line and not after the suggestion.\nscreen:\n%s",
			row, col, want, shown.styledText())
	}
}

// And it never reaches the shell: the line that runs is the line that was
// typed.
func TestAPostdisplayIsNotPartOfTheCommand(t *testing.T) {
	s := newSuggestingSession(t, "\x1b[38;5;8m")
	s.typeLine("echo hi")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] echo hi"+suggestion, "the suggestion drawn")
	s.typeKeys("\r")
	waitFor(t, s.ran, "hi\n", "the command's output")
	if got := s.ran.String(); strings.Contains(got, "sugg") {
		t.Errorf("output = %q, want the suggestion left out of what ran", got)
	}
}

// A keystroke after the one that set it redraws it, and the line that grew is
// still the line: measured, typing another character in that shell draws the
// character and the suggestion after it.
func TestAPostdisplaySurvivesTheNextKeystroke(t *testing.T) {
	s := newSuggestingSession(t, "\x1b[38;5;8m")
	s.typeLine("echo hi")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] echo hi"+suggestion, "the suggestion drawn")
	s.typeKeys("X")
	waitForLine(t, s.screen, "[1] echo hiX"+suggestion, "the suggestion after the line grew")
	shown := s.shown()
	if row, col := shown.at(); col != len("[1] echo hiX") {
		t.Errorf("the cursor is at row %d column %d, want column %d.\nscreen:\n%s",
			row, col, len("[1] echo hiX"), shown.styledText())
	}
}

// And the next line starts without one, which is measured: the widget asked
// about it at the following prompt found it empty.
func TestAPostdisplayDoesNotOutliveTheLine(t *testing.T) {
	s := newSuggestingSession(t, "\x1b[38;5;8m")
	s.typeLine("echo one")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] echo one"+suggestion, "the suggestion drawn")
	s.typeKeys("\r")
	waitFor(t, s.ran, "one\n", "the first command's output")
	s.typeLine("echo two")
	waitForLine(t, s.screen, "[2] echo two", "the second line with no suggestion")
	if got := editedLine(s.screen.String()); strings.Contains(got, "sugg") {
		t.Errorf("the line reads %q, want no suggestion carried over", got)
	}
}

// A postdisplay that pushes the drawn text onto **another row** — which is the
// row that says the geometry counts it.
//
// The four rows above are all short lines, and they cannot catch a redraw that
// measured only the line: the cursor is placed from column zero by an absolute
// move, so on an unwrapped row the answer is the same either way. Mutation
// testing said so — pointing the redraw's arithmetic at the line alone left
// every one of them green (#4217). What tells the two apart is a line whose
// suggestion crosses the edge: the drawn text then ends on the row below, the
// cursor does not, and a redraw that thinks the text ended where the line ended
// puts it on the wrong row.
//
// 74 characters at 80 columns behind a four-cell prompt fills the row to 78, so
// the seven-cell suggestion wraps with two cells to spare.
func TestAPostdisplayThatWrapsLeavesTheCursorOnTheLinesRow(t *testing.T) {
	s := newSuggestingSession(t, "\x1b[38;5;8m")
	line := strings.Repeat("a", 74)
	s.typeLine(line)
	waitForLine(t, s.screen, "[1] "+line, "the long line")
	s.typeKeys("\a")
	// The row the cursor is on is the line's, so editedLine reads the first
	// row: the whole of the line and as much of the suggestion as fits.
	waitForLine(t, s.screen, ("[1] " + line + suggestion)[:fixtureCols], "the wrapped suggestion")

	shown := s.shown()
	rows := strings.Split(shown.text(), "\n")
	if len(rows) < 2 || strings.TrimRight(rows[len(rows)-1], " ") != suggestion[fixtureCols-len("[1] "+line):] {
		t.Errorf("the suggestion did not continue on the row below.\nscreen:\n%s", shown.styledText())
	}
	row, col := shown.at()
	wantRow, wantCol := len(rows)-2, len("[1] "+line)
	if row != wantRow || col != wantCol {
		t.Errorf("the cursor is at row %d column %d, want row %d column %d — the line's own row.\nscreen:\n%s",
			row, col, wantRow, wantCol, shown.styledText())
	}

	// And again through the **whole-line** draw rather than the incremental
	// one, which is a second arithmetic and was reachable by no row above:
	// ^L clears the screen and puts the line back from the top, so the draw
	// that follows it is the one that counts the rows itself. Mutation testing
	// is why this half is here — pointing that draw at the line alone left
	// every other row green, because a cursor placed by an absolute move from
	// column zero lands in the right place on an unwrapped row whatever the
	// end was measured as. Wrapped, the up-move from the end is one row short.
	s.typeKeys("\x0c")
	waitForLine(t, s.screen, ("[1] " + line + suggestion)[:fixtureCols], "the line after a clear")
	shown = s.shown()
	rows = strings.Split(shown.text(), "\n")
	if row, col := shown.at(); row != len(rows)-2 || col != wantCol {
		t.Errorf("after a clear the cursor is at row %d column %d, want row %d column %d.\nscreen:\n%s",
			row, col, len(rows)-2, wantCol, shown.styledText())
	}
}
