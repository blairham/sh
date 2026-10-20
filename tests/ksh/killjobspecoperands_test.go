// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// The operands are read in the order they were written, and the crash is what
// says so (#4666).
//
// ksh93u+ 2012-08-01 **segfaults** on a `%` spec that names no job: measured
// 2026-09-26 on `/bin/ksh`, `kill %99` alone is status 139 with no output at
// all. So whether it carries on past one cannot be asked of it, and
// KillKeepsGoingPastAJobSpecThatNamesNoJob is recorded as unanswered here.
//
// The *ordering* can still be asked, by the one route the crash leaves open:
//
//	kill a %99   `a: Arguments must be %job, process ids, or job pool names`
//	             at status 1 — no crash
//
// A shell that resolved the `%` operand before looking at `a` would have
// crashed before writing that line. The absence of the segfault is the
// measurement, which is why the row is written down rather than derived from
// the neighbors.
func TestThisPresetReadsTheOperandsInOrder(t *testing.T) {
	s := ksh.Semantics()
	if got := s.KillReadsJobSpecsBeforeTheOtherOperands; got != interp.No {
		t.Errorf("KillReadsJobSpecsBeforeTheOtherOperands is %v, want No", got)
	}
	if got := s.KillKeepsGoingPastAJobSpecThatNamesNoJob; got != interp.Unspecified {
		t.Errorf("KillKeepsGoingPastAJobSpecThatNamesNoJob is %v, want it unanswered: the reference crashes", got)
	}
}

// And the shell this preset builds reports the word in front and stops.
func TestKillStopsAtTheWordInFrontOfAMissingJob(t *testing.T) {
	out, _ := answersRun(t, `kill a %99 2>&1 >/dev/null; echo "st=$?"`)
	if !strings.Contains(out, "Arguments must be") {
		t.Errorf("got %q, want the word in front reported", out)
	}
	if strings.Contains(out, "no such job") {
		t.Errorf("got %q: the job spec behind it was reached", out)
	}
	if !strings.Contains(out, "st=1") {
		t.Errorf("got %q, want st=1", out)
	}
}
