// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// This shell stops at the first operand that is not a pid, which is dash's
// answer to #4648 rather than bash's or zsh's.
//
// Measured 2026-09-26 against ksh93u+ 2012-08-01 (`/bin/ksh`, `go version -m`
// → not a Go executable):
//
//	kill a b c           1 `Arguments must be` line   status 1
//	kill a b c d         1 line                       status 1
//	kill a 999999        1 line                       status 1
//	kill 999999 a        2 lines                      status 1
//	kill 999998 999999   2 lines                      status 1
//
// The status is 1 on every row, so it says nothing about which shells stop —
// the line count is the whole measurement here, and `kill 999999 a` is what
// shows that the stop is the malformed word and not a general giving-up: the
// unreachable pid in front of it is reported first.
func TestABadOperandEndsTheKill(t *testing.T) {
	for _, tc := range []struct {
		src   string
		lines int
	}{
		{`kill a b c`, 1},
		{`kill a b c d`, 1},
		{`kill a 999999`, 1},
		{`kill -TERM a b c`, 1},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if n := strings.Count(out, "Arguments must be"); n != tc.lines {
			t.Errorf("%s said %q: %d complaints, want %d", tc.src, out, n, tc.lines)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want status 1", tc.src, out)
		}
	}
}

// The control: a pid that reaches nothing is not this question, so the
// operand behind it is still reached — and the one behind *that* is the
// malformed word that ends it.
func TestAnUnreachablePidDoesNotEndTheKill(t *testing.T) {
	out, _ := answersRun(t, `kill 999999 a 2>&1 >/dev/null; echo "st=$?"`)
	if !strings.Contains(out, "no such process") {
		t.Errorf("got %q, want the unreachable pid reported first", out)
	}
	if n := strings.Count(out, "Arguments must be"); n != 1 {
		t.Errorf("got %q: %d complaints about the word, want one", out, n)
	}
}

func TestThisPresetStopsAtAnOperandThatIsNotAPid(t *testing.T) {
	if got := ksh.Semantics().KillKeepsGoingPastAnOperandThatIsNotAPid; got != interp.No {
		t.Errorf("KillKeepsGoingPastAnOperandThatIsNotAPid is %v, want No", got)
	}
}
