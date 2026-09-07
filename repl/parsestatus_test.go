// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"strings"
	"testing"
)

// A line the parser refuses leaves a status behind, and it is the dialect's.
//
// The complaint alone is not the whole of reporting a failure: `$?` is read by
// the next command, drawn by a prompt with a failure indicator in it and handed
// to the hooks a prompt runs, so a status left where the *previous* command put
// it says "the last thing succeeded" at the exact moment it did not. That is
// the undetectable shape, and it is what these assert against — the status as
// well as the diagnostic, in both loops (#1299).
//
// Measured, through a pseudo-terminal and again with `-i` on a pipe: `false`,
// then a refused line, then `$?` is 2 in bash 5.3 and in the same bash invoked
// as sh, 2 in dash, 3 in ksh93, 1 in zsh, 258 in bash 3.2. Substituting `true`
// for `false` moves no cell, which is what makes it a status being *set*.

// refusedLine is a session run over text, with a dialect that words a parse
// failure one way and gives it one status.
//
// The status is 7, which is not a status anything else in these tests
// produces and not one any panel shell answers with, so a cell holding it can
// only have come from here.
func refusedLine(t *testing.T, text string) (out, errs string, status int) {
	t.Helper()
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	s := Shell{
		Runner: r,
		In:     strings.NewReader(text),
		Out:    &ran,
		Err:    &said,
		Report: func(error) string { return "testsh: refused\n" },
		ParseFailureStatus: func(err error) int {
			if err == nil {
				t.Error("ParseFailureStatus was asked about a nil error")
			}
			return 7
		},
	}
	st, err := s.Run(t.Context())
	if err != nil {
		t.Fatalf("run %q: %v", text, err)
	}
	return ran.String(), said.String(), st
}

func TestARefusedLineSetsTheDialectsStatus(t *testing.T) {
	out, errs, _ := refusedLine(t, "false\nfi\necho \"st=$?\"\n")
	// The whole rendered line, not a substring of it: a diagnostic is what
	// this change writes, and a Contains cannot see a prefix that should not
	// be there.
	if want := "testsh: refused\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if want := "st=7\n"; out != want {
		t.Errorf("output = %q, want %q — the refusal's status, not the one `false` left", out, want)
	}
}

// The same number whatever ran before it, which is what tells a status being
// set from a status being left alone. Without this, `false` ahead of a
// refused line and a dialect answering 1 would read as correct.
func TestTheRefusedStatusDoesNotDependOnWhatRanBefore(t *testing.T) {
	for _, before := range []string{"false", "true", "(exit 5)"} {
		out, _, _ := refusedLine(t, before+"\nfi\necho \"st=$?\"\n")
		if want := "st=7\n"; out != want {
			t.Errorf("after %q: output = %q, want %q", before, out, want)
		}
	}
}

// And the session's own status is the refusal's where nothing ran after it,
// which is the number a front end returns to the process.
func TestASessionThatEndsOnARefusedLineReportsIt(t *testing.T) {
	_, errs, st := refusedLine(t, "true\nfi\n")
	if want := "testsh: refused\n"; errs != want {
		t.Errorf("stderr = %q, want %q", errs, want)
	}
	if st != 7 {
		t.Errorf("session status = %d, want 7", st)
	}
}

// The control, and it is the one that says a broken harness looks different
// from a finding: a line that parses is not touched by any of this, and a
// failing command still reports its own status.
func TestALineThatParsesKeepsItsOwnStatus(t *testing.T) {
	out, errs, _ := refusedLine(t, "false\necho \"st=$?\"\n")
	if errs != "" {
		t.Errorf("stderr = %q, want nothing", errs)
	}
	if want := "st=1\n"; out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// A caller that has not said takes the fallback Report takes, and for the same
// reason: there is no number that is right for every shell, and the panel
// answers this with four different ones. Nothing else about the line moves.
func TestACallerWithNoAnswerLeavesTheStatusAlone(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	s := Shell{
		Runner: r,
		In:     strings.NewReader("false\nfi\necho \"st=$?\"\n"),
		Out:    &ran, Err: &said,
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if said.Len() == 0 {
		t.Error("nothing was said about the refused line")
	}
	if want := "st=1\n"; ran.String() != want {
		t.Errorf("output = %q, want %q — with no answer the status is left where it was", ran.String(), want)
	}
}

// The error the dialect is asked about is the parser's own, so a dialect that
// gives one kind of failure a status of its own can tell them apart. Ours
// does: a `for` whose name is not a name is a run-time failure to bash and a
// parse failure to us, and it carries a different number through both routes.
func TestTheDialectIsAskedAboutTheFailureItGot(t *testing.T) {
	var ran, said strings.Builder
	r := newTestRunner(map[string]string{"PS1": "", "PS2": ""})
	r.Stdout = &ran
	var got error
	s := Shell{
		Runner: r,
		In:     strings.NewReader("fi\necho \"st=$?\"\n"),
		Out:    &ran, Err: &said,
		Report:             func(error) string { return "" },
		ParseFailureStatus: func(err error) int { got = err; return 7 },
	}
	if _, err := s.Run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("the dialect was never asked")
	}
	if errors.Is(got, nil) || got.Error() == "" {
		t.Errorf("the dialect was handed %v, want the parser's own failure", got)
	}
}
