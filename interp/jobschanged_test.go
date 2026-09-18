// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `jobs -n` is a filter on what the shell has already said, which is a
// different question from `-r` and `-s`: those ask what state a job is in and
// this asks whether that state has moved since anybody was told.

// TestJobsListsOnlyWhatChangedSinceTheLastReport pins
// Semantics.JobsListsWhatChangedSinceTheLastReport at all three of the things
// it decides: which jobs the letter keeps, that a second listing keeps none of
// them, and that a shell with nobody to tell has noticed nothing.
func TestJobsListsOnlyWhatChangedSinceTheLastReport(t *testing.T) {
	// A job that is still running and a job that has ended, with a listing
	// in front of the `-n` so there is something for the change to be
	// measured against.
	const src = `set -m
sleep 0.4 &
echo "bang=$!"
jobs >/dev/null
( exit 7 ) &
sleep 0.15
echo "--n--"
jobs -n
echo "--again--"
jobs -n
echo "--end--"
wait
`
	adjust := func(s *Semantics) {
		s.JobsOptions = "lnp"
		s.JobsListsWhatChangedSinceTheLastReport = Yes
		// The case has no terminal, and whether `set -m` needs one is a
		// question of its own — answered here so that a refusal in this file
		// is about the letter under test.
		s.MonitorNeedsATerminal = No
		// And with the monitor on, the closing `wait` reaches the question
		// of what a stopped job does to a wait. Nothing here stops a job;
		// answered so the case does not end on an axis it is not about.
		s.WaitGivesUpOnAStoppedJob = No
	}
	out, st := jobsRun(t, src, adjust)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	first := between(t, out, "--n--", "--again--")
	again := between(t, out, "--again--", "--end--")
	if strings.TrimSpace(first) == "" {
		t.Errorf("the first `jobs -n` listed nothing, want the job whose state moved: %q", out)
	}
	if strings.Contains(first, "bang") {
		t.Errorf("the first `jobs -n` listed %q, want no running job in it", first)
	}
	if strings.TrimSpace(again) != "" {
		t.Errorf("the second `jobs -n` listed %q, want nothing — it was said once", again)
	}

	// The same script with nobody to tell. A shell that is not monitoring
	// has not noticed a job end, so there is no change for the letter to
	// report — and the status is still 0, which is the half that matters to
	// a script.
	quiet, st := jobsRun(t, strings.Replace(src, "set -m\n", "", 1), adjust)
	if st != 0 {
		t.Fatalf("without the monitor: status %d: %s", st, quiet)
	}
	if got := between(t, quiet, "--n--", "--again--"); strings.TrimSpace(got) != "" {
		t.Errorf("without the monitor `jobs -n` listed %q, want nothing", got)
	}
}

// TestJobsDashNIsRefusedWhereTheVectorHasNotAnsweredIt keeps the letter from
// meaning one dialect's thing in another's shell. A vector that hands `jobs`
// the letter and says nothing about what it means gets a refusal rather than
// ksh93's reading.
func TestJobsDashNIsRefusedWhereTheVectorHasNotAnsweredIt(t *testing.T) {
	out, st := jobsRun(t, "sleep 0.1 &\nwait\njobs -n\n", func(s *Semantics) {
		s.JobsOptions = "lnp"
	})
	if st == 0 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("status %d with an unanswered axis, want a refusal: %q", st, out)
	}
}

// between is the text between two markers a case printed.
func between(t *testing.T, out, from, to string) string {
	t.Helper()
	i := strings.Index(out, from)
	j := strings.Index(out, to)
	if i < 0 || j < i {
		t.Fatalf("markers %q and %q not both in %q", from, to, out)
	}
	return out[i+len(from) : j]
}
