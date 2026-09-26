// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// A `%` spec that names no job does not end the builtin here, either way
// round (#4666).
//
// Measured 2026-09-26 against `/opt/homebrew/bin/bash --norc --noprofile` —
// GNU bash 5.3.20(1)-release, `go version -m` on it says it is not a Go
// executable:
//
//	kill %99 a   `%99: no such job` then `a': not a pid or valid job spec`, 1
//	kill a %99   the same two lines the other way round, 1
//
// The order is the order they were written, which is the second axis and is
// asserted below.
func TestKillReportsEveryOperandAroundAMissingJob(t *testing.T) {
	for _, tc := range []struct{ src, first, second string }{
		{`kill %99 a`, "no such job", "not a pid or valid job spec"},
		{`kill a %99`, "not a pid or valid job spec", "no such job"},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
		if len(lines) != 3 {
			t.Errorf("%s said %q: %d lines, want two complaints and the status", tc.src, out, len(lines))
			continue
		}
		if !strings.Contains(lines[0], tc.first) || !strings.Contains(lines[1], tc.second) {
			t.Errorf("%s said %q, want %q then %q", tc.src, out, tc.first, tc.second)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s said %q, want st=1", tc.src, out)
		}
	}
}

func TestThisPresetCarriesOnPastAMissingJobAndReadsTheOperandsInOrder(t *testing.T) {
	s := bash.Semantics()
	if got := s.KillKeepsGoingPastAJobSpecThatNamesNoJob; got != interp.Yes {
		t.Errorf("KillKeepsGoingPastAJobSpecThatNamesNoJob is %v, want Yes", got)
	}
	if got := s.KillReadsJobSpecsBeforeTheOtherOperands; got != interp.No {
		t.Errorf("KillReadsJobSpecsBeforeTheOtherOperands is %v, want No", got)
	}
}
