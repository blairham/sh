// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// This shell reports every bad operand and counts them, which is zsh's answer
// and not dash's (#4648).
//
// It is the row worth measuring rather than deriving. Almost everything this
// applet does with `kill` is the ash family's — the wordings, the single
// status, the missing `-n` — and on this one question it sides with the other
// half of the panel. Measured 2026-09-26 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0, every operand deliberately unusable:
//
//	                       BusyBox 1.37.0        dash 0.5.12
//	kill a b c             3 lines, status 3     1 line,  status 2
//	kill a b c d           4 lines, status 4     1 line,  status 2
//	kill a                 1 line,  status 1     1 line,  status 2
//	kill a 999999          2 lines, status 2     1 line,  status 2
//	kill 999998 999999     2 lines, status 2     2 lines, status 1
//	sleep 20 & kill a $! b 2 lines, status 2     —
//
// The last two rows are the pair the KillStatus half rests on: two failures
// is 2 here and 1 in dash, and a success among the failures does not change
// it. That axis read `KillStatusAnyFailure` here, inherited from the dash
// preset and never measured — invisible until this issue, because the builtin
// stopped at the first malformed operand and a single failure is 1 under
// either policy. `kill 999998 999999` is the row that showed it, and the
// oracle record has said `st=2` for that case all along.
func TestEveryBadOperandIsReportedAndCounted(t *testing.T) {
	for _, tc := range []struct {
		src    string
		lines  int
		status string
	}{
		{`kill a b c`, 3, "st=3"},
		{`kill a b c d`, 4, "st=4"},
		{`kill a`, 1, "st=1"},
		{`kill -TERM a b c`, 3, "st=3"},
	} {
		out, _ := run(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if n := strings.Count(out, "invalid number "); n != tc.lines {
			t.Errorf("%s said %q: %d lines, want %d", tc.src, out, n, tc.lines)
		}
		if !strings.Contains(out, tc.status) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.status)
		}
	}
}

// The count is of failures and not of operands, which is what separates the
// policy this shell keeps from one that simply reports how many targets it
// was given.
func TestTheKillCountIsOfFailuresAndNotOperands(t *testing.T) {
	out, _ := run(t, `kill -0 a $$ b 2>&1 >/dev/null; echo "st=$?"`)
	if n := strings.Count(out, "invalid number "); n != 2 {
		t.Errorf("got %q: %d complaints, want the two bad operands", out, n)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want st=2: one target was reached and two were not", out)
	}
}

// And a pid that is well-formed and reaches nothing counts the same way,
// which is the row that showed the policy was wrong.
func TestTwoUnreachablePidsAreTwo(t *testing.T) {
	out, _ := run(t, `kill 999998 999999 2>&1 >/dev/null; echo "st=$?"`)
	if n := strings.Count(out, "No such process"); n != 2 {
		t.Errorf("got %q: %d complaints, want both pids reported", out, n)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want st=2", out)
	}
}

func TestThisPresetCarriesOnPastABadOperandAndCountsTheFailures(t *testing.T) {
	s := ash.Semantics()
	if got := s.KillKeepsGoingPastAnOperandThatIsNotAPid; got != interp.Yes {
		t.Errorf("KillKeepsGoingPastAnOperandThatIsNotAPid is %v, want Yes", got)
	}
	if got := s.KillStatus; got != interp.KillStatusFailureCount {
		t.Errorf("KillStatus is %v, want the failure count", got)
	}
}
