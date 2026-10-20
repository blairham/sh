// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// This shell stops at the first operand that is not a number, which is the
// other answer to #4648 and the reason it is an axis.
//
// Measured 2026-09-26 against dash 0.5.12 (`/bin/dash`, `go version -m` → not
// a Go executable):
//
//	kill a b c           1 `Illegal number: a` line   status 2
//	kill a b c d         1 line                       status 2
//	kill a 999999        1 line                       status 2
//	kill 999998 999999   2 lines                      status 1
//
// The last row is what makes the rule the malformed *word* rather than a
// failure of any kind: two pids that reach nothing are both reported and the
// status is the ordinary 1. A word that is not a number is an argument fault
// here — 2, this shell's number for a builtin used wrongly — and an argument
// fault ends the builtin.
func TestABadOperandEndsTheKill(t *testing.T) {
	for _, tc := range []struct{ src, status string }{
		{`kill a b c`, "st=2"},
		{`kill a b c d`, "st=2"},
		{`kill a 999999`, "st=2"},
		{`kill -TERM a b c`, "st=2"},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if n := strings.Count(out, "Illegal number"); n != 1 {
			t.Errorf("%s said %q: %d complaints, want exactly the first", tc.src, out, n)
		}
		if !strings.Contains(out, tc.status) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.status)
		}
	}
}

// The control, and the pair that holds the noun fixed: same shape, same
// number of operands, same number of failures — and these words *are*
// numbers, so the builtin runs to the end of the list.
func TestTwoPidsThatReachNothingAreBothReported(t *testing.T) {
	out, _ := answersRun(t, `kill 999998 999999 2>&1 >/dev/null; echo "st=$?"`)
	if n := strings.Count(out, "No such process"); n != 2 {
		t.Errorf("got %q: %d complaints, want both pids reported", out, n)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want st=1: a send that failed is not an argument fault", out)
	}
}

func TestThisPresetStopsAtAnOperandThatIsNotAPid(t *testing.T) {
	if got := dash.Semantics().KillKeepsGoingPastAnOperandThatIsNotAPid; got != interp.No {
		t.Errorf("KillKeepsGoingPastAnOperandThatIsNotAPid is %v, want No", got)
	}
}
