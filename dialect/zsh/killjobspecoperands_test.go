// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A `%` spec that names no job does not end the builtin here, and it counts
// towards the status like any other failed operand (#4666).
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh -f` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` on it says it is not a Go
// executable:
//
//	kill %99 a   `%99: no such job` then `illegal pid: a`, status 2
//	kill a %99   the same two lines the other way round, status 2
//	kill %99     one line, status 1
//
// The third row is what says the 2 is the count of operands that failed and
// not a status this shell keeps for a missing job — the rule
// TestTheKillStatusCountsOperandsAndNotDiagnostics already measured, reaching
// one operand further.
func TestKillReportsEveryOperandAroundAMissingJob(t *testing.T) {
	for _, tc := range []struct {
		src, first, second string
		status             string
	}{
		{`kill %99 a`, "no such job", "illegal pid: a", "st=2"},
		{`kill a %99`, "illegal pid: a", "no such job", "st=2"},
		{`kill %99`, "no such job", "", "st=1"},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		want := 3
		if tc.second == "" {
			want = 2
		}
		if len(lines) != want {
			t.Errorf("%s said %q: %d lines, want %d", tc.src, out, len(lines), want)
			continue
		}
		if !strings.Contains(lines[0], tc.first) {
			t.Errorf("%s said %q, want %q first", tc.src, out, tc.first)
		}
		if tc.second != "" && !strings.Contains(lines[1], tc.second) {
			t.Errorf("%s said %q, want %q second", tc.src, out, tc.second)
		}
		if !strings.Contains(out, tc.status) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.status)
		}
	}
}

func TestThisPresetCarriesOnPastAMissingJobAndReadsTheOperandsInOrder(t *testing.T) {
	s := zsh.Semantics()
	if got := s.KillKeepsGoingPastAJobSpecThatNamesNoJob; got != interp.Yes {
		t.Errorf("KillKeepsGoingPastAJobSpecThatNamesNoJob is %v, want Yes", got)
	}
	if got := s.KillReadsJobSpecsBeforeTheOtherOperands; got != interp.No {
		t.Errorf("KillReadsJobSpecsBeforeTheOtherOperands is %v, want No", got)
	}
}
