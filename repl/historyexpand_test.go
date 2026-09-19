// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// expandingShell is a session with just enough in it to ask the one question:
// a Runner with the state on, and somewhere for the echo to land.
func expandingShell(t *testing.T, on bool) (Shell, *strings.Builder) {
	t.Helper()
	r := newTestRunner(nil)
	r.SetHistoryExpansion(on)
	var errs strings.Builder
	return Shell{Name: "bash", Runner: r, Err: &errs}, &errs
}

// The line reaches the parser expanded, and the expanded line is echoed — both
// halves, because a shell that expanded silently would run a command nobody
// saw. Measured on bash 5.3.20 through a pseudo-terminal with a two-row
// prompt: `echo one two three` then `echo !!` writes `echo echo one two three`
// to standard error and then prints `echo one two three`.
func TestAnExpandedLineIsRewrittenAndEchoed(t *testing.T) {
	s, errs := expandingShell(t, true)
	got, outcome := s.expanded("echo !!", []string{"echo one two three"}, nil)
	if outcome != runLine {
		t.Fatalf("outcome = %v, want the line run", outcome)
	}
	if want := "echo echo one two three"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	if want := "echo echo one two three\n"; errs.String() != want {
		t.Errorf("echoed %q, want %q", errs.String(), want)
	}
}

// And a line the expansion did not change is echoed nothing at all, which is
// the measured half that says the echo is about the rewrite rather than about
// the feature being on.
func TestAnUnchangedLineIsNotEchoed(t *testing.T) {
	s, errs := expandingShell(t, true)
	got, outcome := s.expanded("echo plain", []string{"echo one"}, nil)
	if outcome != runLine || got != "echo plain" {
		t.Fatalf("line = %q outcome = %v", got, outcome)
	}
	if errs.String() != "" {
		t.Errorf("echoed %q, want nothing", errs.String())
	}
}

// With the state off nothing is looked at, which is what `set +H` buys and
// what the three dialects without an expander get. The line reaching the
// parser unchanged is the assertion; a shell that expanded anyway would be
// answering a different command from the one that was typed.
func TestWithTheStateOffTheLineIsUntouched(t *testing.T) {
	s, errs := expandingShell(t, false)
	got, outcome := s.expanded("echo !!", []string{"echo one two three"}, nil)
	if outcome != runLine || got != "echo !!" {
		t.Errorf("line = %q outcome = %v, want the line untouched", got, outcome)
	}
	if errs.String() != "" {
		t.Errorf("echoed %q, want nothing", errs.String())
	}
}

// A reference nothing matches stops the line and says so in this dialect's
// words. Measured: `bash: !nosuch: event not found`, and the line does not
// run — which is the half a shell that only printed a warning would get wrong.
func TestAReferenceNothingMatchesStopsTheLine(t *testing.T) {
	s, errs := expandingShell(t, true)
	got, outcome := s.expanded("echo !nosuch", []string{"echo one"}, nil)
	if outcome != dropLine {
		t.Errorf("outcome = %v and the line was %q, want it abandoned", outcome, got)
	}
	if want := "bash: !nosuch: event not found\n"; errs.String() != want {
		t.Errorf("said %q, want %q", errs.String(), want)
	}
}

// `:p` remembers the expansion and runs nothing, which is the whole of what
// makes it the safe way to look at a reference before trusting it.
func TestThePrintModifierRemembersAndRunsNothing(t *testing.T) {
	s, _ := expandingShell(t, true)
	var remembered []string
	got, outcome := s.expanded("!!:p", []string{"rm -rf /tmp/scratch"}, func(l string) {
		remembered = append(remembered, l)
	})
	if outcome != dropLine {
		t.Errorf("outcome = %v and the line was %q, want it abandoned", outcome, got)
	}
	if len(remembered) != 1 || remembered[0] != "rm -rf /tmp/scratch" {
		t.Errorf("remembered %q, want the expansion", remembered)
	}
}

// A blank line is not a reference to anything, and asking the engine about one
// would be asking it to index an empty string. The guard is cheap and the
// failure it prevents is a complaint at every bare press of return.
func TestABlankLineIsNotExpanded(t *testing.T) {
	s, errs := expandingShell(t, true)
	if got, outcome := s.expanded("   ", nil, nil); outcome != runLine || got != "   " {
		t.Errorf("line = %q outcome = %v", got, outcome)
	}
	if errs.String() != "" {
		t.Errorf("said %q about a blank line", errs.String())
	}
}

// With `shopt histverify` on, the same expansion is handed back instead of
// run, and the echo the off state writes is **replaced** rather than added to.
//
// The echo is the half worth asserting here rather than only through a
// terminal: measured on bash 5.3.20, a verified line writes nothing to
// standard error, because the person is looking at the text on their own line.
// A shell that seeded the line and echoed it too would show it twice.
func TestAVerifiedExpansionIsHandedBackAndNotEchoed(t *testing.T) {
	s, errs := expandingShell(t, true)
	s.Runner.SetHistoryExpansionVerifies(true)
	var remembered []string
	got, outcome := s.expanded("echo !!", []string{"echo one two three"}, func(l string) {
		remembered = append(remembered, l)
	})
	if outcome != verifyLine {
		t.Fatalf("outcome = %v, want the line handed back", outcome)
	}
	if want := "echo echo one two three"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	if errs.String() != "" {
		t.Errorf("echoed %q, want nothing", errs.String())
	}
	// And nothing is recorded by the verification itself: measured, a `!!`
	// abandoned with ^C leaves the list exactly as it was, so the option is
	// not `:p` and the ordinary accept below it does all the remembering.
	if len(remembered) != 0 {
		t.Errorf("remembered %q, want nothing", remembered)
	}
}

// `:p` is unmoved by the option, which is the row that says the two routes
// that run nothing are not the same route.
//
// Measured on bash 5.3.20 with `shopt -s histverify`: `!!:p` prints the
// expansion and joins the list, exactly as it does with the option off, and
// nothing is drawn back on the line.
func TestThePrintModifierIsUnmovedByVerification(t *testing.T) {
	s, errs := expandingShell(t, true)
	s.Runner.SetHistoryExpansionVerifies(true)
	var remembered []string
	_, outcome := s.expanded("!!:p", []string{"rm -rf /tmp/scratch"}, func(l string) {
		remembered = append(remembered, l)
	})
	if outcome != dropLine {
		t.Errorf("outcome = %v, want the line abandoned", outcome)
	}
	if want := "rm -rf /tmp/scratch\n"; errs.String() != want {
		t.Errorf("printed %q, want %q", errs.String(), want)
	}
	if len(remembered) != 1 || remembered[0] != "rm -rf /tmp/scratch" {
		t.Errorf("remembered %q, want the expansion", remembered)
	}
}
