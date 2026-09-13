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
