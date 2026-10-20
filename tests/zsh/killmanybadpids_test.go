// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"

	"github.com/blairham/sh/dialect/zsh"
)

// `kill a b c` reports every operand and reports how many failed (#4648).
//
// Measured 2026-09-26 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `go
// version -m` → not a Go executable), each operand deliberately unusable so
// that nothing could be delivered:
//
//	kill a b c           3 lines   status 3
//	kill a b c d         4 lines   status 4
//	kill a b c d e       5 lines   status 5
//	kill a a a           3 lines   status 3
//	kill a               1 line    status 1
//	kill a 999999        2 lines   status 2
//	kill -TERM a b c     3 lines   status 3
//	kill -NOPE a b       2 lines   status 1
//
// The last two rows are the pair that says what the status is keyed on. The
// signal is named in both and neither delivers anything, so the *signal* is
// not what decides; what decides is whether an operand was ever looked at. A
// signal this shell cannot name is refused with the listing hint behind it —
// two lines, and 1, because the target list was never reached.
//
// This is the chunk `B11kill.ztst` stops on, named there *kill with multiple
// wrong inputs should increment status*.
func TestEveryIllegalPidIsReportedAndCounted(t *testing.T) {
	for _, tc := range []struct {
		src    string
		lines  int
		status int
	}{
		{`kill a b c`, 3, 3},
		{`kill a b c d`, 4, 4},
		{`kill a b c d e`, 5, 5},
		{`kill a a a`, 3, 3},
		{`kill a`, 1, 1},
		{`kill -TERM a b c`, 3, 3},
		{`kill -INT a b c`, 3, 3},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		lines := strings.Count(out, "illegal pid: ")
		if lines != tc.lines {
			t.Errorf("%s said %q: %d illegal-pid lines, want %d", tc.src, out, lines, tc.lines)
		}
		if want := "st=" + strconv.Itoa(tc.status); !strings.Contains(out, want) {
			t.Errorf("%s said %q, want %q", tc.src, out, want)
		}
	}
}

// The status counts the **operands** that failed and not the lines written,
// which is the one pair where those two numbers part.
//
// Both rows write two lines to stderr. `kill a b` is two operands that could
// not be read, and is 2. `kill -NOPE a b` is a signal this shell cannot name,
// refused with its listing hint behind it, before any operand is looked at —
// two lines again, and 1. A rule keyed on the diagnostics would give them the
// same number.
func TestTheKillStatusCountsOperandsAndNotDiagnostics(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`kill a b`, "st=2"},
		{`kill -NOPE a b`, "st=1"},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if n := strings.Count(out, "\n"); n != 3 {
			t.Errorf("%s said %q: %d lines, want two complaints and the status", tc.src, out, n)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.want)
		}
	}
}

// And the two axes this shell answers to get there, stated as values so that
// a preset edit that takes either back fails here rather than in a suite run.
func TestThisPresetCarriesOnPastABadOperandAndCountsTheFailures(t *testing.T) {
	s := zsh.Semantics()
	if got := s.KillKeepsGoingPastAnOperandThatIsNotAPid; got != interp.Yes {
		t.Errorf("KillKeepsGoingPastAnOperandThatIsNotAPid is %v, want Yes", got)
	}
	if got := s.KillStatus; got != interp.KillStatusFailureCount {
		t.Errorf("KillStatus is %v, want the failure count", got)
	}
}
