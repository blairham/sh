// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/interp"
)

// This applet reads the `%` operands before the others, and a spec that names
// no job ends the builtin (#4666).
//
// It is the column the two axes were split for. Carrying on past a word that
// is not a pid is **yes** here — see TestEveryBadOperandIsReportedAndCounted —
// and carrying on past a missing job is **no**, which is a pattern no single
// value can hold.
//
// Measured 2026-09-26 in
// alpine@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b,
// BusyBox v1.37.0:
//
//	kill %99 a               `%99: no such job` alone, status 2
//	kill a %99               the same line alone, status 2 — `a` is never
//	                         looked at
//	kill %99 %98             `%99: no such job` alone, status 2
//	kill %99                 the same line, status 2
//	kill a                   `invalid number 'a'`, status 1
//	sleep 30 & kill $! %99   the job's line alone, status 2, and a `jobs`
//	                         after it still lists that job as Running
//
// The last two rows are the ones that keep the change honest. `kill a` at 1
// says the 2 belongs to the missing job rather than to this shell's `kill`;
// and the live process in front of the spec receiving nothing says the rule
// is about the whole builtin rather than about which line comes first.
func TestAMissingJobIsReadFirstAndEndsTheBuiltin(t *testing.T) {
	for _, tc := range []struct{ src, want, absent, status string }{
		{`kill %99 a`, "%99: no such job", "invalid number", "st=2"},
		{`kill a %99`, "%99: no such job", "invalid number", "st=2"},
		{`kill %99 %98`, "%99: no such job", "%98", "st=2"},
		{`kill %99`, "%99: no such job", "invalid number", "st=2"},
		{`kill a`, "invalid number 'a'", "no such job", "st=1"},
	} {
		out, _ := run(t, tc.src+` 2>&1 >/dev/null; echo "st=$?"`)
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.want)
		}
		if strings.Contains(out, tc.absent) {
			t.Errorf("%s said %q, want nothing about %q", tc.src, out, tc.absent)
		}
		if !strings.Contains(out, tc.status) {
			t.Errorf("%s said %q, want %q", tc.src, out, tc.status)
		}
	}
}

// And nothing in front of the spec is signaled.
//
// Signal 0 at the shell itself, which is certainly there: this preset's status
// is the count of operands that failed, so a run that delivered to `$$` would
// still report 1 — and the row that discriminates is the **absence** of the
// send, read through KillStatus rather than through the output, because both
// readings write the same one line.
func TestAMissingJobIsReadBeforeAnythingIsSignaled(t *testing.T) {
	out, _ := run(t, `kill -0 $$ %99 2>&1 >/dev/null; echo "st=$?"`)
	if n := strings.Count(out, "no such job"); n != 1 {
		t.Errorf("got %q: %d job complaints, want one", out, n)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("got %q, want st=2 — the job's own status, not a count with a success in it", out)
	}
}

func TestThisPresetReadsJobSpecsFirstAndStopsAtAMissingOne(t *testing.T) {
	s := ash.Semantics()
	if got := s.KillReadsJobSpecsBeforeTheOtherOperands; got != interp.Yes {
		t.Errorf("KillReadsJobSpecsBeforeTheOtherOperands is %v, want Yes", got)
	}
	if got := s.KillKeepsGoingPastAJobSpecThatNamesNoJob; got != interp.No {
		t.Errorf("KillKeepsGoingPastAJobSpecThatNamesNoJob is %v, want No", got)
	}
	if got := ash.Diagnostics().KillNoSuchJobStatus; got != 2 {
		t.Errorf("KillNoSuchJobStatus is %d, want 2", got)
	}
}
