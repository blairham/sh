// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// An option a running script may not move and the command line that started
// the shell may: a route split inside one shell rather than a disagreement
// between two, which is why it is exercised here — the front end is the only
// thing that knows which route a request came by.
//
// Named by the axes, which is the rule outside dialect/: the shell under test
// is one that has the `-t` letter, refuses to move it
// (Diagnostics.ImmovableOptionLetters) and answers
// Semantics.ImmovableOptionsSetAtInvocation.
//
// immovableAtInvocation builds it, with the invocation answer handed in so
// both sides of the axis are reachable.
func immovableAtInvocation(out, errs *strings.Builder, atInvocation interp.Answer) driver.Shell {
	sem := interp.PosixSemantics()
	// The letter exists and a script may not write it, which is what makes
	// the invocation a separate question rather than the same one.
	sem.SetHasTheTLetter = interp.No
	sem.ImmovableOptionsSetAtInvocation = atInvocation
	sem.OneCommandStopsACommandString = interp.No
	return driver.Shell{
		Name:        "testsh",
		Dialect:     syntax.Core(),
		Semantics:   sem,
		Diagnostics: interp.Diagnostics{ImmovableOptionLetters: map[string]string{"set": "t"}},
		Stdout:      out,
		Stderr:      errs,
	}
}

// TestAnImmovableOptionLetterIsStillTakenAtTheInvocation: the front end's `-t`
// turns the option on and one line of the script runs, where the same letter
// written in that script is refused.
func TestAnImmovableOptionLetterIsStillTakenAtTheInvocation(t *testing.T) {
	script := scriptAt(t, "echo P1\necho P2\necho P3\n")
	var out, errs strings.Builder
	sh := immovableAtInvocation(&out, &errs, interp.Yes)
	if code := driver.MainArgs(sh, []string{"testsh", "-t", script}); code != 0 {
		t.Errorf("status %d, want 0 (stderr %q)", code, errs.String())
	}
	if out.String() != "P1\n" || errs.String() != "" {
		t.Errorf("ran %q / said %q, want P1 and nothing said", out.String(), errs.String())
	}
}

// TestTheSameLetterIsStillRefusedInsideTheScript: the grant is the route's
// and not the letter's, so nothing about a running script has moved. The
// refusal is said and the option does not turn on — the second line runs,
// which it would not have if the letter had been taken. Whether the refusal
// also *ends* the script is a dialect's own answer and is exercised where
// that answer lives.
func TestTheSameLetterIsStillRefusedInsideTheScript(t *testing.T) {
	var out, errs strings.Builder
	sh := immovableAtInvocation(&out, &errs, interp.Yes)
	driver.MainArgs(sh, []string{"testsh", scriptAt(t, "set -t\necho ON\necho OFF\n")})
	if errs.String() == "" {
		t.Errorf("said nothing, want the refusal")
	}
	if out.String() != "ON\nOFF\n" {
		t.Errorf("ran %q, want both lines — the option must not have moved", out.String())
	}
}

// TestWithoutTheAxisTheInvocationIsRefusedToo: a shell that has not answered
// keeps the refusal on both routes, which is what makes this an axis rather
// than a rule — the majority of the panel either moves the option everywhere
// or has never heard of the letter.
func TestWithoutTheAxisTheInvocationIsRefusedToo(t *testing.T) {
	script := scriptAt(t, "echo P1\necho P2\n")
	var out, errs strings.Builder
	sh := immovableAtInvocation(&out, &errs, interp.Unspecified)
	if code := driver.MainArgs(sh, []string{"testsh", "-t", script}); code == 0 {
		t.Errorf("status 0, want a refusal")
	}
	if out.String() != "" {
		t.Errorf("ran %q, want nothing", out.String())
	}
}
