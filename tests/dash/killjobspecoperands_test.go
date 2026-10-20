// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/interp"
)

// A `%` spec that names no job ends the builtin here, and so does a word that
// is not a pid — so the operands are read in the order they were written and
// the first fault is the only one reported (#4666).
//
// Measured 2026-09-26 against `/bin/dash` — 0.5.12:
//
//	kill %99 a   `kill: No such job: %99` alone, status 2
//	kill a %99   `kill: Illegal number: a` alone, status 2
//
// The pair is what separates the two axes from each other here: this shell
// answers No to both, so each row is a different complaint and neither shell
// reaches the second operand.
func TestKillStopsAtTheFirstFaultAroundAMissingJob(t *testing.T) {
	for _, tc := range []struct{ src, want, absent string }{
		{`kill %99 a`, "No such job: %99", "Illegal number"},
		{`kill a %99`, "Illegal number: a", "No such job"},
	} {
		out, _ := answersRun(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, tc.absent) {
			t.Errorf("%s said %q: it went on to the second operand", tc.src, out)
		}
		if !strings.Contains(out, "st=2") {
			t.Errorf("%s said %q, want st=2", tc.src, out)
		}
	}
}

func TestThisPresetStopsAtAMissingJobAndReadsTheOperandsInOrder(t *testing.T) {
	s := dash.Semantics()
	if got := s.KillKeepsGoingPastAJobSpecThatNamesNoJob; got != interp.No {
		t.Errorf("KillKeepsGoingPastAJobSpecThatNamesNoJob is %v, want No", got)
	}
	if got := s.KillReadsJobSpecsBeforeTheOtherOperands; got != interp.No {
		t.Errorf("KillReadsJobSpecsBeforeTheOtherOperands is %v, want No", got)
	}
}
