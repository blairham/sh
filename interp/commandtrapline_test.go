// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// DEBUG and ERR fire *at a command*, and one dialect numbers their bodies
// from that command's line where it numbers every other trap body from the
// body's own first line. That is a second axis and not a refinement of
// TrapBodyLine: knowing a dialect's answer to one says nothing about the
// other, and this shell gave the first answer to both — so a DEBUG body's
// `$LINENO` was 1 wherever it fired, and every diagnostic from inside one
// named a line of the body rather than a line of the script.
//
// The bodies here are two lines for the reason trapline_test.go gives: a
// one-line body cannot tell "the body's first line" from "wherever it
// fired", and a one-line probe is why this looked like one question.

// commandTrapSem is the answers a DEBUG or ERR body needs before the line
// rules are reachable at all.
func commandTrapSem(s *Semantics) {
	s.TrapHasDebugCondition = Yes
	s.TrapHasErrCondition = Yes
	s.DebugTrapRunsInsideCalls = No
	s.DebugTrapRunsInSubshells = No
	s.ErrTrapRunsInsideFunctions = No
	s.ErrTrapRunsInSubshells = No
}

// lineNamed is the line a failure inside a trap body reported.
func lineNamed(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	_, errs, _ := trapRun(t, src, func(s *Semantics) {
		commandTrapSem(s)
		set(s)
	}, Diagnostics{Location: LocationLineWord})
	for _, line := range strings.Split(errs, "\n") {
		if strings.Contains(line, "nosuchcmd-xyz") {
			return line
		}
	}
	return "(no diagnostic: " + errs + ")"
}

// echoed is what the body printed, which is where `$LINENO` shows up.
func echoed(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	out, _, _ := trapRun(t, src, func(s *Semantics) {
		commandTrapSem(s)
		set(s)
	}, Diagnostics{Location: LocationLineWord})
	return strings.TrimSpace(out)
}

// The trap is set on lines 1 and 2 and fires before `echo two` on line 3, so
// a body counted from the firing line puts `$LINENO` at 3 and the failure on
// its second line at 4, while a body counted from its own first line puts
// them at 1 and 2.
const debugBody = "trap 'echo at=$LINENO\nnosuchcmd-xyz' DEBUG\necho two"

// The failing command is on line 4, so the same two readings give 4 and 5
// against 1 and 2.
const errBody = "trap 'echo at=$LINENO\nnosuchcmd-xyz' ERR\necho two\nfalse"

func within(s *Semantics)   { s.CommandTrapBodyLine = TrapBodyLineWithin }
func offset(s *Semantics)   { s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired }
func pinnedAt(s *Semantics) { s.CommandTrapBodyLine = TrapBodyLineWhereItFired }

// TestADebugBodyCanCountFromWhereItFired is bash's answer, and the one that
// was missing: the body's own first line is the line the command about to
// run is written on.
func TestADebugBodyCanCountFromWhereItFired(t *testing.T) {
	if got := echoed(t, debugBody, offset); !strings.Contains(got, "at=3") {
		t.Errorf("$LINENO = %q, want the line the trap fired before", got)
	}
	if got := lineNamed(t, debugBody, offset); !strings.Contains(got, "line 4:") {
		t.Errorf("got %q, want the firing line plus the body's", got)
	}
}

// TestADebugBodyCanCountItsOwnLines is the other answer, kept reachable
// because it is what a dialect with no second opinion leaves in place.
func TestADebugBodyCanCountItsOwnLines(t *testing.T) {
	if got := echoed(t, debugBody, within); !strings.Contains(got, "at=1") {
		t.Errorf("$LINENO = %q, want the body's first line", got)
	}
	if got := lineNamed(t, debugBody, within); !strings.Contains(got, "line 2:") {
		t.Errorf("got %q, want the body's second line", got)
	}
}

// TestADebugBodyCanNameOnlyWhereItFired is zsh's reading, where the body's
// own lines say nothing at all.
func TestADebugBodyCanNameOnlyWhereItFired(t *testing.T) {
	if got := lineNamed(t, debugBody, pinnedAt); !strings.Contains(got, "line 3:") {
		t.Errorf("got %q, want where it fired and nothing else", got)
	}
}

// TestAnErrBodyIsCountedTheSameWay, because the axis is about the two
// conditions that fire at a command and not about DEBUG alone.
func TestAnErrBodyIsCountedTheSameWay(t *testing.T) {
	if got := echoed(t, errBody, offset); !strings.Contains(got, "at=4") {
		t.Errorf("$LINENO = %q, want the line that failed", got)
	}
	if got := lineNamed(t, errBody, offset); !strings.Contains(got, "line 5:") {
		t.Errorf("got %q, want the failing line plus the body's", got)
	}
	if got := lineNamed(t, errBody, within); !strings.Contains(got, "line 2:") {
		t.Errorf("got %q, want the body's second line", got)
	}
}

// TestTheTwoTrapLineQuestionsAreSeparate is the whole reason for the second
// field: one script, both kinds of trap, one dialect's pair of answers. A
// single axis could not produce these two lines at once.
func TestTheTwoTrapLineQuestionsAreSeparate(t *testing.T) {
	src := "trap 'echo sig=$LINENO' INT\ntrap 'echo dbg=$LINENO' DEBUG\nkill -INT $$"
	got := echoed(t, src, func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineWithin
		s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
	})
	if !strings.Contains(got, "dbg=3") {
		t.Errorf("output %q: want the DEBUG body counted from the firing line", got)
	}
	if !strings.Contains(got, "sig=1") {
		t.Errorf("output %q: want the signal body counted from its own first line", got)
	}
}

// TestTheCommandTrapStyleDoesNotOutlastTheBody. The flag that says which
// question a body is asking is the firing site's, and a trap that fires
// after it must be numbered by its own rule — the first version of this
// left the flag set, and every EXIT trap in the rest of the script was
// numbered as though it had fired at a command.
func TestTheCommandTrapStyleDoesNotOutlastTheBody(t *testing.T) {
	src := "trap 'echo dbg=$LINENO' DEBUG\necho two\ntrap - DEBUG\ntrap 'echo exit=$LINENO' EXIT"
	got := echoed(t, src, func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineWithin
		s.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
		s.ExitTrapFiresPastTheEnd = No
	})
	if !strings.Contains(got, "exit=1") {
		t.Errorf("output %q: want the EXIT body counted by its own rule", got)
	}
}
