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
	got, ok := s.expanded("echo !!", []string{"echo one two three"}, nil)
	if !ok {
		t.Fatal("the line was abandoned")
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
	got, ok := s.expanded("echo plain", []string{"echo one"}, nil)
	if !ok || got != "echo plain" {
		t.Fatalf("line = %q ok = %v", got, ok)
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
	got, ok := s.expanded("echo !!", []string{"echo one two three"}, nil)
	if !ok || got != "echo !!" {
		t.Errorf("line = %q ok = %v, want the line untouched", got, ok)
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
	got, ok := s.expanded("echo !nosuch", []string{"echo one"}, nil)
	if ok {
		t.Errorf("the line ran anyway, as %q", got)
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
	got, ok := s.expanded("!!:p", []string{"rm -rf /tmp/scratch"}, func(l string) {
		remembered = append(remembered, l)
	})
	if ok {
		t.Errorf("the line ran, as %q", got)
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
	if got, ok := s.expanded("   ", nil, nil); !ok || got != "   " {
		t.Errorf("line = %q ok = %v", got, ok)
	}
	if errs.String() != "" {
		t.Errorf("said %q about a blank line", errs.String())
	}
}
