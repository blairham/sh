// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// The round trip an action outside the editor gets: the line out, the line
// back.
//
// Nothing here names a shell, for the reason bindings_test.go gives — a
// dialect imports this package, so what a widget function is called and what
// the line arrives in are asserted in dialect/zsh, next to the measurement
// they came from. This pins what the *seam* does with whatever comes back.

// typedThroughShell runs a line through an editor whose ^G is bound to an
// action the shell performs, with run standing in for the dialect.
func typedThroughShell(t *testing.T, run func(Line) (Line, bool), keys string) string {
	t.Helper()
	var out strings.Builder
	s := Shell{
		KeyBindings: func() map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
	}
	if run != nil {
		s.RunWidget = func(_ context.Context, name string, in Line) (Line, bool) {
			if name != "w" {
				t.Errorf("action name = %q, want %q", name, "w")
			}
			return run(in)
		}
	}
	e := s.newEditor(t.Context())
	e.in, e.out = strings.NewReader(keys), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// TestAShellActionIsGivenTheLineAndItsAnswerIsTaken is the whole capability. A
// key nobody would otherwise act on rewrites the line, because something
// outside the editor said so.
func TestAShellActionIsGivenTheLineAndItsAnswerIsTaken(t *testing.T) {
	var saw Line
	got := typedThroughShell(t, func(in Line) (Line, bool) {
		saw = in
		return Line{Buffer: "rewritten", Cursor: 3}, true
	}, "abc\a\n")
	if saw != (Line{Buffer: "abc", Cursor: 3}) {
		t.Errorf("the action was given %+v, want the typed line with the cursor at its end", saw)
	}
	if want := "rewritten"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// And the cursor came back where the action put it, which the line alone
	// cannot show: three more characters typed land after the third.
	if got, want := typedThroughShell(t, func(Line) (Line, bool) {
		return Line{Buffer: "abcdef", Cursor: 3}, true
	}, "x\aZZ\n"), "abcZZdef"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// The cursor is put back in range rather than trusted, because an action that
// walked off the end is asking for the end and a slice would panic.
func TestACursorOutOfRangeIsBroughtBack(t *testing.T) {
	for _, c := range []struct {
		name  string
		at    int
		typed string
		want  string
	}{
		{"past the end", 99, "x\aZ\n", "abcZ"},
		{"before the start", -5, "x\aZ\n", "Zabc"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := typedThroughShell(t, func(Line) (Line, bool) {
				return Line{Buffer: "abc", Cursor: c.at}, true
			}, c.typed)
			if got != c.want {
				t.Errorf("line = %q, want %q", got, c.want)
			}
		})
	}
}

// An action the shell will not run leaves the line exactly as it was, and the
// key is still claimed — the bytes are not typed into the line instead.
func TestAnActionTheShellDeclinesLeavesTheLineAlone(t *testing.T) {
	got := typedThroughShell(t, func(Line) (Line, bool) {
		return Line{Buffer: "clobbered", Cursor: 0}, false
	}, "abc\a\n")
	if want := "abc"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// And a session whose front end offered no way to run one: the key does
// nothing, rather than the byte reaching the editor's own dispatch. A binding
// is a binding whether or not anything can perform it.
func TestWithNoWayToRunOneTheKeyDoesNothing(t *testing.T) {
	if got, want := typedThroughShell(t, nil, "abc\a\n"), "abc"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// A panic in an action costs the keystroke and not the session, which is the
// stronger version of the reason a typed line is guarded: this one runs on a
// key somebody may have pressed by accident.
func TestAPanickingActionDoesNotEndTheLine(t *testing.T) {
	var errs strings.Builder
	var out strings.Builder
	s := Shell{
		Name: "sh", Err: &errs,
		KeyBindings: func() map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
		RunWidget: func(context.Context, string, Line) (Line, bool) {
			panic("in a widget")
		},
	}
	e := s.newEditor(t.Context())
	e.in, e.out = strings.NewReader("abc\aZ\n"), &out
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("readLine: %v", err)
	}
	if want := "abcZ"; line != want {
		t.Errorf("line = %q, want %q", line, want)
	}
	if !strings.Contains(errs.String(), "in a widget") {
		t.Errorf("diagnostics = %q, want the panic reported", errs.String())
	}
}

// What a session does with a shell that has work put aside for a time: it asks
// once per prompt, before the prompt hook, and puts the status back afterwards.
func TestElapsedWorkRunsBeforeThePromptHookAndKeepsTheStatus(t *testing.T) {
	s, out := hookShell(t, hooksLikeZsh())
	define(t, s.Runner, "precmd", `echo hook`)
	calls := 0
	s.RunScheduled = func(context.Context) {
		calls++
		s.Runner.SetExitStatus(9)
		_, _ = out.WriteString("scheduled\n")
	}
	s.Runner.SetExitStatus(3)
	s.hooks = &hookState{reported: map[string]bool{}}
	s.runElapsed(t.Context())
	s.fireBeforePrompt(t.Context(), false)
	if calls != 1 {
		t.Errorf("asked %d times, want once", calls)
	}
	if got, want := out.String(), "scheduled\nhook\n"; got != want {
		t.Errorf("order = %q, want %q", got, want)
	}
	if got := s.Runner.ExitStatus(); got != 3 {
		t.Errorf("status after = %d, want 3 — what a scheduled command left must not reach the next command", got)
	}
}

// The same two facts through a whole session rather than through one call,
// which is what pins the *place* the ask is made from: `beforeReading` runs it
// before the prompt hook and inside the hook's own line discipline, and a
// straight-line reading of runElapsed cannot show that.
func TestScheduledWorkRunsBeforeThePromptHookInASession(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("echo typed\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeZsh(),
		RunScheduled: func(context.Context) { _, _ = out.WriteString("scheduled\n") },
	}
	define(t, r, "precmd", `echo hook`)
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatalf("running: %v", err)
	}
	// Two prompts: one before the typed line and one after it.
	want := "scheduled\nhook\ntyped\nscheduled\nhook\n"
	if got := out.String(); got != want {
		t.Errorf("the session ran %q, want %q", got, want)
	}
}

// And a scheduled command that called `exit` ends the session with the status
// it set, the way a prompt hook that did does — the status is not put back and
// no prompt is drawn.
func TestScheduledWorkThatExitsEndsTheSession(t *testing.T) {
	var out, errs strings.Builder
	r := newTestRunner(map[string]string{"PS1": "RDY> "})
	r.Stdout, r.Stderr = &out, &out
	s := Shell{
		Runner: r, In: strings.NewReader("echo unreachable\n"),
		Out: &out, Err: &errs, Name: "sh", Hooks: hooksLikeZsh(),
	}
	define(t, r, "scheduled_thing", `echo bye; exit 3`)
	s.RunScheduled = func(ctx context.Context) {
		_, _ = s.Runner.CallFunction(ctx, "scheduled_thing")
	}
	status, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	if status != 3 {
		t.Errorf("the session ended with %d, want 3", status)
	}
	if got := out.String(); got != "bye\n" {
		t.Errorf("the session ran %q, want the scheduled command alone", got)
	}
	if errs.String() != "" {
		t.Errorf("a prompt was drawn: %q", errs.String())
	}
}

// And a session with nothing scheduled asks nothing, which is what keeps this
// off the cost of every prompt in three of the four dialects.
func TestASessionWithNothingScheduledAsksNothing(t *testing.T) {
	s, _ := hookShell(t, HookStyle{})
	s.RunScheduled = nil
	s.runElapsed(t.Context()) // must not panic on a nil field
}
