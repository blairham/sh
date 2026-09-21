// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A segment computed outside this process, on a real screen.
//
// The unit tests beside this one drive the seam directly, which says nothing
// about whether a *session* ever wires one — and a segment that works in a
// test and not in a shell is the blind spot this package has had before.
//
// **A two-row prompt**, and that is the whole reason this file exists rather
// than one more unit test. The rows above the line are written once and
// never again: every keystroke rewrites the last row alone, which is what
// stops an Up arrow leaving a ladder of prompts behind it. A segment that
// arrives on the upper row is exactly the case a one-row prompt cannot tell
// apart from a working one — the pieces can each be right while the nesting
// is wrong, and only the screen shows it.

// newSourceSession is a session drawing a two-row themed prompt whose upper
// row is a segment from outside the binary.
func newSourceSession(t *testing.T) (*session, *fakeSource) {
	t.Helper()
	source := newFakeSource("plugin weather", "weather")
	s := newSessionWith(t, func(sh *Shell) {
		// The last row carries the command number, because the fixture waits
		// for a prompt that has not been drawn before and a theme replaces
		// the parameter that would otherwise carry one. Drawn by a session
		// function so that the mark is the theme's own output rather than
		// something written beside it.
		line := 0
		sh.StartLine = func() {
			line++
			sh.Runner.SetVar("PROMPTS", itoa(line))
		}
		sh.Runner.SetVar("SH_PROMPT_LEFT_ELEMENTS", "weather newline mark")
		sh.Runner.SetVar("SH_PROMPT_ICONS", "none")
		if !sh.Runner.DefineFunction("mark", `printf '[%s]' "$PROMPTS"`) {
			t.Fatal("the function would not define")
		}
		theme := NewTheme(sh.Runner.GetVar)
		theme.Consult(source)
		sh.Theme = theme
	})
	return s, source
}

// The upper row is replaced where it stands when the segment arrives, and
// the line being typed is still there underneath it.
func TestASegmentFromOutsideTheBinaryRedrawsTheRowAboveTheLine(t *testing.T) {
	s, source := newSourceSession(t)
	s.typeLine("echo hello")

	if got := s.row(0); strings.Contains(got, "OUTSIDE") {
		t.Fatalf("the segment drew before it had answered:\n%q", got)
	}
	source.arrived("weather", PromptSegment{Content: "OUTSIDE-ARRIVED"})
	waitFor(t, s.screen, "OUTSIDE-ARRIVED", "the segment that arrived")

	if got := s.row(0); !strings.Contains(got, "OUTSIDE-ARRIVED") {
		t.Errorf("the upper row was not redrawn:\n%q", got)
	}
	if got := s.row(1); !strings.Contains(got, "echo hello") {
		t.Errorf("the typed line did not survive the redraw:\n%q", got)
	}
	// And exactly one copy is on the screen. A redraw that rewrote the upper
	// row from the last row — which is where a keystroke's redraw begins —
	// would leave the old one above it, which is the ladder.
	if n := strings.Count(s.shown().text(), "OUTSIDE-ARRIVED"); n != 1 {
		t.Errorf("%d copies of the segment are on the screen:\n%s", n, s.shown().text())
	}

	s.typeKeys("\n")
	waitFor(t, s.ran, "hello", "the command's output")
	s.end()
}

// And the source is told what the prompt is being drawn for, by the session
// rather than by a test calling DrawPrompt.
func TestARealSessionTellsASourceWhatThePromptIsFor(t *testing.T) {
	s, source := newSourceSession(t)
	s.typeLine("echo told\n")
	waitFor(t, s.ran, "told", "the command's output")

	told := source.contexts()
	if len(told) == 0 {
		t.Fatal("a real session never told the source anything")
	}
	// The terminal's width, which the session measures and no test wrote
	// down: a context of zero values would satisfy "it was told something"
	// and prove nothing about what it was told.
	if told[0].Columns != fixtureCols {
		t.Errorf("the session told the source %d columns, want %d: %+v",
			told[0].Columns, fixtureCols, told[0])
	}
	s.end()
}
