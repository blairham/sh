// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A trap's body has lines of its own, and which lines a failure inside it
// names is three answers. These name the axis rather than the shell, and all
// of them use a *two-line* body: a one-line body cannot tell "the body's
// first line" from "wherever it fired", which is how this looked like one
// answer for as long as the probes were one-liners.

// trapLineOf runs a script whose trap body fails and returns the line the
// failure named.
func trapLineOf(t *testing.T, src string, set func(*Semantics)) string {
	t.Helper()
	full := func(s *Semantics) {
		s.TrapParsesOptions = Yes
		s.TrapOneArgumentIsACondition = Yes
		if set != nil {
			set(s)
		}
	}
	_, errs, _ := trapRun(t, src, full, Diagnostics{Location: LocationLineWord})
	for _, line := range strings.Split(errs, "\n") {
		if strings.Contains(line, "nosuchcmd-xyz") {
			return line
		}
	}
	return "(no diagnostic: " + errs + ")"
}

const exitBody = "echo one\ntrap 'echo a\nnosuchcmd-xyz' EXIT\necho two"

// TestATrapBodyCountsItsOwnLines is the answer two dialects give, and the
// one this had before the question was one.
func TestATrapBodyCountsItsOwnLines(t *testing.T) {
	got := trapLineOf(t, exitBody, nil)
	if !strings.Contains(got, "line 2:") {
		t.Errorf("got %q, want the body's second line", got)
	}
}

// TestATrapBodyCanBeCountedFromWhereItFired is ksh93's, and the EXIT trap
// counts as firing on the first line there — which is why this answer is
// invisible on EXIT and shows only on a signal.
func TestATrapBodyCanBeCountedFromWhereItFired(t *testing.T) {
	offset := func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineOffsetFromWhereItFired
		s.ExitTrapFiresPastTheEnd = No
	}
	if got := trapLineOf(t, exitBody, offset); !strings.Contains(got, "line 2:") {
		t.Errorf("EXIT: got %q, want the body's second line", got)
	}

	// Fired from line 3, second line of the body, so 3 + 2 - 1.
	signal := "trap 'echo a\nnosuchcmd-xyz' INT\necho two\nkill -INT $$"
	if got := trapLineOf(t, signal, offset); !strings.Contains(got, "line 5:") {
		t.Errorf("signal: got %q, want the firing line plus the body's", got)
	}
}

// TestATrapBodyCanNameOnlyWhereItFired is zsh's: every line of the body
// reports the same number, so the body's own lines say nothing.
func TestATrapBodyCanNameOnlyWhereItFired(t *testing.T) {
	pinned := func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineWhereItFired
		s.ExitTrapFiresPastTheEnd = Yes
	}
	// Four lines, so the EXIT trap fires on the fifth.
	if got := trapLineOf(t, exitBody, pinned); !strings.Contains(got, "line 5:") {
		t.Errorf("EXIT: got %q, want the line after the script's last", got)
	}

	signal := "trap 'echo a\nnosuchcmd-xyz' INT\necho two\nkill -INT $$"
	if got := trapLineOf(t, signal, pinned); !strings.Contains(got, "line 4:") {
		t.Errorf("signal: got %q, want where it fired and nothing else", got)
	}
}

// TestWhereTheExitTrapFiredIsItsOwnQuestion, and it is only ever asked by a
// dialect whose style needs a firing line at all.
func TestWhereTheExitTrapFiredIsItsOwnQuestion(t *testing.T) {
	first := trapLineOf(t, exitBody, func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineWhereItFired
		s.ExitTrapFiresPastTheEnd = No
	})
	past := trapLineOf(t, exitBody, func(s *Semantics) {
		s.TrapBodyLine = TrapBodyLineWhereItFired
		s.ExitTrapFiresPastTheEnd = Yes
	})
	if !strings.Contains(first, "line 1:") {
		t.Errorf("counted from the first line: got %q", first)
	}
	if !strings.Contains(past, "line 5:") {
		t.Errorf("counted past the end: got %q", past)
	}
	if first == past {
		t.Error("both answers produced the same line, so neither is being chosen")
	}
}

// TestTheBodysLinesAreGivenBackAfterwards, because a trap fires in the
// middle of a script and everything after it must name its own lines again.
func TestTheBodysLinesAreGivenBackAfterwards(t *testing.T) {
	pinned := func(s *Semantics) {
		s.TrapParsesOptions = Yes
		s.TrapBodyLine = TrapBodyLineWhereItFired
	}
	src := "trap 'nosuchcmd-xyz' INT\nkill -INT $$\nalso-nosuchcmd-xyz"
	_, errs, _ := trapRun(t, src, pinned, Diagnostics{Location: LocationLineWord})
	if !strings.Contains(errs, "line 3: also-nosuchcmd-xyz") {
		t.Errorf("stderr = %q, want the line after the trap to name its own line", errs)
	}
}
