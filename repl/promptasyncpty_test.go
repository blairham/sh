// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// A prompt drawn again, mid-line, because something it draws arrived.
//
// **A two-row prompt throughout**, and that is the whole reason this file is
// a terminal test rather than a unit one. The rows above the line are written
// once and never again — every keystroke rewrites the last row alone, which
// is what stops an Up arrow leaving a ladder of prompts behind it (#2467) —
// so a segment on the upper row is precisely the case a one-row prompt cannot
// tell apart from a working one. The pieces can each be right while the
// nesting is wrong, and only the screen shows it.
//
// The assertions are over the **screen** — what a terminal of this width
// would be showing — for the reason screenmodel_test.go gives.

// asyncTheme is a theme whose upper row changes when something publishes.
type asyncTheme struct {
	mu      sync.Mutex
	upper   string
	line    int
	renders int
	publish func()
}

// rendered is how many prompts this theme has drawn, which is one per line
// plus one per wake that was served. A test that has to know the wake was
// *served* rather than merely raised waits on this: the screen cannot say
// so, since the whole claim under test is that nothing was written.
func (a *asyncTheme) rendered() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.renders
}

// PublishTo satisfies PromptPublisher.
func (a *asyncTheme) PublishTo(publish func()) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.publish = publish
}

// arrived is the segment's answer turning up on some other goroutine, which
// is the only thing an asynchronous segment ever does.
func (a *asyncTheme) arrived(upper string) {
	a.mu.Lock()
	a.upper = upper
	publish := a.publish
	a.mu.Unlock()
	if publish != nil {
		publish()
	}
}

// startedLine is the once-per-prompt-line counter the fixture waits on. It is
// counted here rather than in DrawPrompt because DrawPrompt is called again
// for a redraw, and a mark that moved on a redraw would tell the harness a
// fresh prompt had been drawn when none had.
func (a *asyncTheme) startedLine() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.line++
}

func (a *asyncTheme) DrawPrompt(info PromptInfo) (ThemedPrompt, bool) {
	if info.Continued {
		return ThemedPrompt{Cont: "> "}, true
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.renders++
	return ThemedPrompt{Text: "upper " + a.upper + "\n[" + itoa(a.line) + "] "}, true
}

func newAsyncSession(t *testing.T) (*session, *asyncTheme) {
	t.Helper()
	theme := &asyncTheme{upper: "waiting"}
	s := newSessionWith(t, func(sh *Shell) {
		sh.Theme = theme
		sh.StartLine = theme.startedLine
	})
	return s, theme
}

// The upper row is replaced where it stands, and the line being typed is
// still there underneath it.
func TestASegmentThatArrivesRedrawsTheRowAboveTheLine(t *testing.T) {
	s, theme := newAsyncSession(t)
	s.typeLine("echo hello")

	if got := s.row(0); !strings.Contains(got, "upper waiting") {
		t.Fatalf("the prompt did not start out waiting:\n%q", got)
	}
	theme.arrived("ARRIVED")
	waitFor(t, s.screen, "ARRIVED", "the segment that arrived")

	if got := s.row(0); !strings.Contains(got, "upper ARRIVED") {
		t.Errorf("the upper row was not redrawn:\n%q", got)
	}
	if got := s.row(1); !strings.Contains(got, "echo hello") {
		t.Errorf("the typed line did not survive the redraw:\n%q", got)
	}
	// And exactly one prompt is on the screen. A redraw that rewrote the
	// upper row from the last row — which is where a keystroke's redraw
	// begins — would leave the old one above it, which is the ladder.
	if n := strings.Count(s.shown().text(), "upper "); n != 1 {
		t.Errorf("%d copies of the prompt's upper row are on the screen:\n%s", n, s.shown().text())
	}

	s.typeKeys("\n")
	s.end()
}

// Typing goes on working afterwards: the editor's idea of where the cursor is
// has to survive a redraw it did not start.
func TestTypingContinuesAfterASegmentArrives(t *testing.T) {
	s, theme := newAsyncSession(t)
	s.typeLine("echo one")
	theme.arrived("ARRIVED")
	waitFor(t, s.screen, "ARRIVED", "the segment that arrived")

	s.typeKeys(" two")
	if got := s.row(1); !strings.Contains(got, "echo one two") {
		t.Errorf("typing after the redraw drew %q", got)
	}
	s.typeKeys("\n")
	waitFor(t, s.ran, "one two", "the command's output")
	s.end()
}

// A publisher is allowed to be wrong. Publishing when nothing would be drawn
// differently writes nothing to the screen at all, which is what makes a
// publisher that cannot tell cheap rather than flickering.
func TestPublishingWithNothingNewWritesNothing(t *testing.T) {
	s, theme := newAsyncSession(t)
	s.typeLine("echo hello")
	// One publish first, so the wake is known to be live and the screen is
	// known to be settled before the one under test.
	theme.arrived("ARRIVED")
	waitFor(t, s.screen, "ARRIVED", "the segment that arrived")

	before, renders := s.screen.String(), theme.rendered()
	theme.arrived("ARRIVED")
	// Waiting on the *render* and not on the screen, because the claim is
	// that the screen does not change: a wake that was raised and never
	// served would make this test pass for the wrong reason.
	for waited := 0; waited < 2000 && theme.rendered() == renders; waited++ {
		time.Sleep(time.Millisecond)
	}
	if theme.rendered() == renders {
		t.Fatal("the wake was never served, so this proves nothing")
	}
	if after := s.screen.String(); after != before {
		t.Errorf("a publish that changed nothing wrote %q", strings.TrimPrefix(after, before))
	}

	s.typeKeys("\n")
	s.end()
}

// A segment written as a shell function, in a real session, through the
// wiring a session actually does.
//
// The unit tests beside this one hand the theme a resolver directly, which
// says nothing about whether a session ever installs one — and a segment that
// works in a test and not in a shell is the blind spot this package has had
// before. So the prompt here *is* the function's output: the mark the fixture
// waits for is drawn by shell code, and nothing else draws it.
func TestAShellFunctionDrawsInARealSession(t *testing.T) {
	s := newSessionWith(t, func(sh *Shell) {
		line := 0
		sh.StartLine = func() {
			line++
			sh.Runner.SetVar("PROMPTS", itoa(line))
		}
		sh.Runner.SetVar("SH_PROMPT_LEFT_ELEMENTS", "mark prompt_char")
		sh.Runner.SetVar("SH_PROMPT_ICONS", "none")
		if !sh.Runner.DefineFunction("mark", `printf '[%s]' "$PROMPTS"`) {
			t.Fatal("the function would not define")
		}
		sh.Theme = NewTheme(sh.Runner.GetVar)
	})

	// typeLine waits for `[1]`, which only the shell function can have drawn.
	s.typeLine("echo one-two\n")
	waitFor(t, s.ran, "one-two", "the command's output")
	s.end()
}
