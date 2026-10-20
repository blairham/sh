// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `kill a b c` reports every operand here too, and the status is 1 (#4648).
//
// The issue was filed against zsh and the wrong half of it would have been to
// fix zsh alone: the carrying-on is this shell's as well, and only the
// *number* differs. Measured 2026-09-26 against bash 5.3.20
// (`/opt/homebrew/bin/bash --norc --noprofile`, `go version -m` → not a Go
// executable):
//
//	kill a b c        3 `not a pid or valid job spec` lines   status 1
//	kill a b c d      4 lines                                 status 1
//	kill a            1 line                                  status 1
//	kill a 999999     2 lines                                 status 1
//	kill -NOPE a b    1 line                                  status 1
//
// **1 on every row, which is why the line count is the measurement and the
// status is not.** This shell's KillStatus is "0 if anything was signaled",
// and nothing was signaled in any row above, so the status cannot tell a
// shell that stopped at `a` from one that reported all three. A grid read off
// the status alone would have put bash with ksh and dash.
func TestEveryOperandThatIsNotAPidIsReported(t *testing.T) {
	for _, tc := range []struct {
		src   string
		lines int
	}{
		{`kill a b c`, 3},
		{`kill a b c d`, 4},
		{`kill a`, 1},
		{`kill -TERM a b c`, 3},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if n := strings.Count(out, "not a pid or valid job spec"); n != tc.lines {
			t.Errorf("%s said %q: %d lines, want %d", tc.src, out, n, tc.lines)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", tc.src, out)
		}
	}
}

// The status is 0 where something *was* signaled, which is the row that tells
// this shell's policy from the count zsh and BusyBox ash keep.
//
// Measured: `sleep 20 & kill a $! b` is two complaints and 0 in bash 5.3.20,
// with the sleep gone. Signal 0 here so that nothing is delivered and the row
// is about the status alone.
func TestASignaledTargetAmongBadOnesIsStatusZero(t *testing.T) {
	out, _ := answersRun(t, `kill -0 a $$ b 2>&1 >/dev/null; echo "st=$?"`)
	if n := strings.Count(out, "not a pid or valid job spec"); n != 2 {
		t.Errorf("got %q: %d complaints, want the two bad operands", out, n)
	}
	if !strings.Contains(out, "st=0") {
		t.Errorf("got %q, want status 0: a target was signaled", out)
	}
}

func TestThisPresetCarriesOnPastAnOperandThatIsNotAPid(t *testing.T) {
	s := bash.Semantics()
	if got := s.KillKeepsGoingPastAnOperandThatIsNotAPid; got != interp.Yes {
		t.Errorf("KillKeepsGoingPastAnOperandThatIsNotAPid is %v, want Yes", got)
	}
	if got := s.KillStatus; got != interp.KillStatusAnySuccess {
		t.Errorf("KillStatus is %v, want any success", got)
	}
}
