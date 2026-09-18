// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A trap body is a script of its own, and running it walks the shell's idea
// of "the line I am on" through the body's lines. That line belongs to the
// script, so the body has to give it back.
//
// The DEBUG condition is where the damage is certain rather than likely: it
// fires *before* the command it is tracing, so the very next thing to read
// `$LINENO` is that command, and without the restore it reads the last line
// of the trap body instead. Measured on bash 5.3.15 — a DEBUG body whose
// action is a two-line function leaves the traced command reporting its own
// line, where this engine reported the body's.
//
// The body here calls a function on purpose. A body of plain commands walks
// the line only as far as its own length, which a one-line body cannot show
// at all; a call walks it into another block entirely, which is what makes
// the difference visible however short the body is.

func debugLineSem(s *Semantics) {
	s.TrapHasDebugCondition = Yes
	s.DebugTrapRunsInsideCalls = No
	s.DebugTrapRunsInSubshells = No
}

// The trap is set on line 1 and its action calls `deep`, whose body is on
// lines 3 and 4. `echo at=$LINENO` is on line 6, so the line it reports is 6
// if the body gave the line back and 4 if it did not.
const debugThenLineno = "trap 'deep' DEBUG\n" +
	"deep() {\n" +
	"  :\n" +
	"  :\n" +
	"}\n" +
	"echo at=$LINENO"

func TestATrapBodyGivesBackTheLineTheScriptWasOn(t *testing.T) {
	out, _, _ := trapRun(t, debugThenLineno, debugLineSem, Diagnostics{Location: LocationLineWord})
	if got := strings.TrimSpace(out); !strings.Contains(got, "at=6") {
		t.Errorf("$LINENO after a DEBUG body = %q, want the traced command's own line 6", got)
	}
}

// The control: with no trap set at all, the same line reports the same
// number. Without this the test above would pass on an engine that reported
// some fixed number for `$LINENO` everywhere.
func TestTheLineIsTheSameWithNoTrapAtAll(t *testing.T) {
	src := "deep() {\n  :\n  :\n}\n:\necho at=$LINENO"
	out, _, _ := trapRun(t, src, debugLineSem, Diagnostics{Location: LocationLineWord})
	if got := strings.TrimSpace(out); !strings.Contains(got, "at=6") {
		t.Errorf("$LINENO with no trap = %q, want 6", got)
	}
}

// The RETURN trap and the DEBUG body: the one body `set -T` does not carry
// this trap into.
//
// Measured on bash 5.3.15, that build invoked as `sh`, and bash 3.2. With the
// trap set at the top level and the shell tracing calls, a function called
// from an ERR body, an EXIT body or a signal body fires it; one called from a
// DEBUG body fires nothing. A function that sets the trap in its own body
// fires it from inside a DEBUG body like anywhere else, which is what says
// the exemption is about the carriage and not about the body.
//
// A correction rather than an axis: zsh, ksh93, dash and ash refuse
// `trap … RETURN` outright, so no other column has the condition to ask.
//
// The size of it is why it earns a test. A DEBUG body runs before *every*
// command, so a traced script with both traps set carried two extra lines of
// output per command for the whole of its run.
func returnAndDebugSem(s *Semantics) {
	debugLineSem(s)
	s.TrapHasReturnCondition = Yes
	s.TrapHasErrCondition = Yes
	s.ErrTrapRunsInsideFunctions = No
	s.ErrTrapRunsInSubshells = No
	// `set -T` is what carries either trap into a call, and the letters are
	// bash's — the dialect this behavior was measured on.
	s.SetHasTheErrtraceLetter = Yes
}

// `act` does not set the trap itself, so the only thing that could reach it
// is the carriage `set -T` turns on. `false` is what the ERR arrangement
// needs and what the DEBUG arrangement traces.
const carriedIntoABody = "set -T\n" +
	"act() { echo ran; }\n" +
	"trap 'echo RET' RETURN\n" +
	"trap 'act' COND\n" +
	"false\n" +
	"trap - COND\n"

// The same, with the trap set in the function's own body, where no carriage
// is involved at all.
const setInsideTheFunction = "set -T\n" +
	"act() { trap 'echo RET' RETURN; echo ran; }\n" +
	"trap 'act' COND\n" +
	"false\n" +
	"trap - COND\n"

func firedFrom(t *testing.T, src, cond string) string {
	t.Helper()
	out, _, _ := trapRun(t, strings.ReplaceAll(src, "COND", cond), returnAndDebugSem,
		Diagnostics{Location: LocationLineWord})
	return out
}

func TestTheReturnTrapIsNotCarriedIntoTheDebugBody(t *testing.T) {
	out := firedFrom(t, carriedIntoABody, "DEBUG")
	if strings.Contains(out, "RET") {
		t.Errorf("a carried trap in the DEBUG body printed %q, want no RETURN in it", out)
	}
	if !strings.Contains(out, "ran") {
		t.Errorf("the DEBUG body printed %q, want the call itself to have run", out)
	}
}

// The control that says the exemption is DEBUG's and not every body's.
func TestTheReturnTrapIsCarriedIntoTheErrBody(t *testing.T) {
	if out := firedFrom(t, carriedIntoABody, "ERR"); !strings.Contains(out, "RET") {
		t.Errorf("a carried trap in the ERR body printed %q, want RETURN in it", out)
	}
}

// The control that says the exemption is about the carriage and not about
// the body: a function holding the trap itself fires it from inside DEBUG.
func TestATrapTheFunctionSetItselfFiresInsideTheDebugBody(t *testing.T) {
	if out := firedFrom(t, setInsideTheFunction, "DEBUG"); !strings.Contains(out, "RET") {
		t.Errorf("a trap the function set itself printed %q, want RETURN in it", out)
	}
}

// Where a RETURN counts as having *fired*, which became a question the day
// the condition joined DEBUG and ERR in being numbered from there — see
// Semantics.CommandTrapBodyLine. Before that every RETURN body reported 1
// and 2 whatever the script looked like, so nothing here could be wrong.
//
// Measured 2026-09-13 on bash 5.3.15 and bash 3.2, which are the only columns
// with the condition at all. Three arrangements and two answers:
//
//	a `return` on line 7 of a body opening on line 5   7 and 8
//	a body opening on line 5 that falls off its end    5 and 6
//	a one-line file sourced by a `.` on line 4         4 and 5
//
// So it is the `return`'s own line wherever in the body it stands, the line
// the *body opened* on when the call fell off the end, and the line of the
// `.` for a sourced file. Not the body's last command in any of the three,
// which is where the line record happens to be sitting and is the answer this
// engine would have given for free.
func returnLineSem(s *Semantics) {
	s.TrapHasReturnCondition = Yes
	s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
	// The carriage, so a trap set at the top level reaches a call whose body
	// did not set one. bash's letters, which is the dialect this was
	// measured on.
	s.SetHasTheErrtraceLetter = Yes
}

func returnLines(t *testing.T, src string) string {
	t.Helper()
	out, errs, _ := trapRun(t, src, returnLineSem, Diagnostics{Location: LocationLineWord})
	if errs != "" {
		t.Fatalf("ran %q: stderr %q", src, errs)
	}
	return out
}

// The body opens on line 5 and the `return` is on line 7.
const returnFromLineSeven = "set -T\n" +
	"trap 'echo at=$LINENO\necho at=$LINENO' RETURN\n" +
	"f()\n" +
	"{\n" +
	"  echo in\n" +
	"  return\n" +
	"}\n" +
	"f\n"

// The same shape with no `return` in it, so the call falls off the end. The
// body still opens on line 5 and its last command is on line 6, which is what
// makes the two answers tell each other apart.
const returnByFallingOffTheEnd = "set -T\n" +
	"trap 'echo at=$LINENO\necho at=$LINENO' RETURN\n" +
	"f()\n" +
	"{\n" +
	"  echo in\n" +
	"  echo last\n" +
	"}\n" +
	"f\n"

func TestAnExplicitReturnFiresAtItsOwnLine(t *testing.T) {
	if got, want := returnLines(t, returnFromLineSeven), "in\nat=7\nat=8\n"; got != want {
		t.Errorf("a `return` on line 7: %q, want %q", got, want)
	}
}

func TestFallingOffTheEndFiresAtTheLineTheBodyOpenedOn(t *testing.T) {
	if got, want := returnLines(t, returnByFallingOffTheEnd), "in\nlast\nat=5\nat=6\n"; got != want {
		t.Errorf("a body that fell off its end at line 6: %q, want %q", got, want)
	}
}
