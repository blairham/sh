// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// Text drawn **before** the line without being part of it: postdisplaypty_test's
// other half, and on the same two-row prompt for the same reason. #6414.
//
// Measured 2026-10-15 in tmux against zsh 5.9.2, prompt `UPPER` over `> `, a
// `self-insert` replacement setting PREDISPLAY="pre:": typing `e` draws
// `> pre:e` with the cursor in column 7, `x` after it `> pre:ex`, ^A puts the
// cursor in column 6 — after the predisplay, never on it — the accepted
// command is `e`, and the next prompt starts without one.
const predisplay = "pre:"

// newPrefixingSession is a session whose `w` action sets a predisplay of text.
func newPrefixingSession(t *testing.T, text string) *session {
	t.Helper()
	drawn := 0
	return newSessionWith(t, func(sh *Shell) {
		sh.Theme = PromptThemeFunc(func(info PromptInfo) (ThemedPrompt, bool) {
			if info.Continued {
				return ThemedPrompt{Cont: "> "}, true
			}
			drawn++
			return ThemedPrompt{Text: "upper\n[" + itoa(drawn) + "] "}, true
		})
		sh.KeyBindings = func(Keymap) map[string]Binding {
			return map[string]Binding{"\a": {Function: "w"}}
		}
		sh.RunWidget = func(_ context.Context, _ string, in Line) (Line, bool) {
			in.Predisplay = text
			return in, true
		}
	})
}

// It is drawn between the prompt and the line, and the cursor is moved past it
// with the line: after the line's end, and at the line's start after ^A.
func TestAPredisplayIsDrawnBeforeTheLine(t *testing.T) {
	s := newPrefixingSession(t, predisplay)
	s.typeLine("echo hi")
	waitForLine(t, s.screen, "[1] echo hi", "the line typed")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] "+predisplay+"echo hi", "the predisplay drawn")
	shown := s.shown()
	if row, col := shown.at(); col != len("[1] "+predisplay+"echo hi") {
		t.Errorf("the cursor is at row %d column %d, want column %d — after the line, past the predisplay.\nscreen:\n%s",
			row, col, len("[1] "+predisplay+"echo hi"), shown.styledText())
	}
	s.typeKeys("X")
	waitForLine(t, s.screen, "[1] "+predisplay+"echo hiX", "the line grown after it")
	s.typeKeys("\x01")
	// The cursor's own move: wait for it, since the screen's text does not
	// change.
	want := len("[1] " + predisplay)
	waitUntil(t, "the cursor at the line's start", func() bool {
		_, col := s.shown().at()
		return col == want
	})
}

// And it never reaches the shell, and the next line starts without one.
func TestAPredisplayIsNotPartOfTheCommandAndDoesNotOutliveTheLine(t *testing.T) {
	s := newPrefixingSession(t, predisplay)
	s.typeLine("echo one")
	s.typeKeys("\a")
	waitForLine(t, s.screen, "[1] "+predisplay+"echo one", "the predisplay drawn")
	s.typeKeys("\r")
	waitFor(t, s.ran, "one\n", "the first command's output")
	if got := s.ran.String(); strings.Contains(got, "pre") {
		t.Errorf("output = %q, want the predisplay left out of what ran", got)
	}
	s.typeLine("echo two")
	waitForLine(t, s.screen, "[2] echo two", "the second line with no predisplay")
}

// One that pushes the line onto **another row**: the row that says the
// geometry counts it, since on an unwrapped row an absolute move from column
// zero lands in the same place whichever arithmetic placed it. Through the
// incremental draw and then through the whole-line one, after ^L.
//
// 74 cells of predisplay at 80 columns behind a four-cell prompt fills the row
// to 78, so `echo hi` starts there and wraps after `ec`.
func TestAPredisplayThatWrapsCarriesTheCursorOntoTheNextRow(t *testing.T) {
	long := strings.Repeat("p", 74)
	s := newPrefixingSession(t, long)
	s.typeLine("echo hi")
	waitForLine(t, s.screen, "[1] echo hi", "the line typed")
	s.typeKeys("\a")
	wantTail := ("[1] " + long + "echo hi")[fixtureCols:]
	// The row the cursor is on reads the wrapped tail, and the cursor is at
	// its end: the line followed the predisplay onto the row below.
	check := func(when string) {
		t.Helper()
		waitUntil(t, when, func() bool {
			_, col := s.shown().at()
			return editedLine(s.screen.String()) == wantTail && col == len(wantTail)
		})
		shown := s.shown()
		rows := strings.Split(shown.text(), "\n")
		row, _ := shown.at()
		if row == 0 || !strings.HasPrefix(rows[row-1], "[1] "+long) {
			t.Errorf("%s: the row above the cursor is not the prompt and the predisplay.\nscreen:\n%s", when, shown.styledText())
		}
	}
	check("the wrapped line")
	s.typeKeys("\x0c")
	check("the wrapped line after a clear")
}
