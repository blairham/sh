// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Job specs by command text, `wait`'s answers about jobs that are not
// there, `wait -n`, and what `disown` lets go of. Background jobs here are
// compound commands — they need no process and no PATH — and their command
// text is what `%name` resolves against.

// TestJobSpecsResolveByCommandText: `%name` is a prefix of the command,
// `%?text` a substring, and a spec that is no number is a missing job in
// the dialect that resolves only numbers.
func TestJobSpecsResolveByCommandText(t *testing.T) {
	src := "{ exit 3; } &\nwait %{\necho st=$?"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		s.JobSpecsByName = Yes
		s.WaitReportsAMissingJob = Yes
	}, Diagnostics{})
	if !strings.Contains(out, "st=3") || errs != "" {
		t.Errorf("stdout %q stderr %q, want the named job waited for", out, errs)
	}

	out, _, _ = declRun(t, "{ exit 4; } &\nwait '%?xit'\necho st=$?", func(s *Semantics) {
		s.JobSpecsByName = Yes
		s.WaitReportsAMissingJob = Yes
	}, Diagnostics{})
	if !strings.Contains(out, "st=4") {
		t.Errorf("stdout = %q, want the substring spec resolved", out)
	}

	out, errs, _ = declRun(t, "{ exit 3; } &\nwait %{\necho st=$?", func(s *Semantics) {
		s.JobSpecsByName = No
		s.WaitReportsAMissingJob = Yes
	}, Diagnostics{WaitNoSuchJob: "wait: not here: %[1]s", WaitNoSuchJobStatus: 2})
	if !strings.Contains(errs, "wait: not here: %{") || !strings.Contains(out, "st=2") {
		t.Errorf("stdout %q stderr %q, want the spec refused as a missing job", out, errs)
	}
}

// TestAmbiguousJobNames: a second match is refused by one answer and taken —
// the most recent — by the other.
func TestAmbiguousJobNames(t *testing.T) {
	src := "{ exit 3; } &\n{ exit 4; } &\nwait %{\necho st=$?"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		s.JobSpecsByName = Yes
		s.AmbiguousJobNameIsRefused = Yes
	}, Diagnostics{AmbiguousJobSpec: "%[1]s: %[2]s: two of those"})
	if !strings.Contains(errs, "wait: {: two of those") {
		t.Errorf("stderr = %q, want the refusal with the %% stripped", errs)
	}
	if !strings.Contains(out, "st=127") {
		t.Errorf("stdout = %q, want the missing-job status", out)
	}

	out, errs, _ = declRun(t, src, func(s *Semantics) {
		s.JobSpecsByName = Yes
		s.AmbiguousJobNameIsRefused = No
	}, Diagnostics{})
	if !strings.Contains(out, "st=4") || errs != "" {
		t.Errorf("stdout %q stderr %q, want the most recent match taken", out, errs)
	}
}

// TestWaitReportsAMissingJob, or does not: one engine says nothing at all
// and reports 0.
func TestWaitReportsAMissingJob(t *testing.T) {
	out, errs, _ := declRun(t, "wait %5\necho st=$?", func(s *Semantics) {
		s.WaitReportsAMissingJob = No
	}, Diagnostics{})
	if !strings.Contains(out, "st=0") || errs != "" {
		t.Errorf("stdout %q stderr %q, want silence and 0", out, errs)
	}
	out, errs, _ = declRun(t, "wait %5\necho st=$?", func(s *Semantics) {
		s.WaitReportsAMissingJob = Yes
	}, Diagnostics{})
	if !strings.Contains(errs, "wait: %5: no such job") || !strings.Contains(out, "st=127") {
		t.Errorf("stdout %q stderr %q, want the complaint and 127", out, errs)
	}
}

// TestWaitNWaitsForTheNextJob: the first finisher's status, the job
// forgotten, and 127 in silence with nothing to wait for.
func TestWaitNWaitsForTheNextJob(t *testing.T) {
	src := "{ exit 3; } &\nwait -n\necho st=$?\nwait -n\necho again=$?"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		s.WaitNWaitsForTheNextJob = Yes
	}, Diagnostics{})
	if !strings.Contains(out, "st=3") {
		t.Errorf("stdout = %q, want the finished job's status", out)
	}
	if !strings.Contains(out, "again=127") || errs != "" {
		t.Errorf("stdout %q stderr %q, want the second -n to find nothing, silently", out, errs)
	}
}

// TestDisownLetsGoOrOnlyShields: one answer takes the job out of the
// table, the other leaves the listing alone; and a bare disown with no
// current job speaks only where the dialect gave it words.
func TestDisownLetsGoOrOnlyShields(t *testing.T) {
	sem := func(remove Answer) func(*Semantics) {
		return func(s *Semantics) {
			s.DisownRemovesTheJob = remove
			s.JobsListFinishedJobs = Yes
			s.JobsListNewestFirst = No
			s.JobsShowBackgroundCommand = No
		}
	}
	src := "{ exit 0; } &\ndisown\njobs\necho st=$?"
	out, _, _ := declRun(t, src, sem(Yes), Diagnostics{})
	if strings.Contains(out, "[1]") {
		t.Errorf("stdout = %q, want the job gone from the listing", out)
	}
	out, _, _ = declRun(t, src, sem(No), Diagnostics{})
	if !strings.Contains(out, "[1]") {
		t.Errorf("stdout = %q, want the job still listed", out)
	}

	out, errs, _ := declRun(t, "disown\necho st=$?", nil, Diagnostics{})
	if errs != "" || !strings.Contains(out, "st=1") {
		t.Errorf("stdout %q stderr %q, want a silent 1 with no wording given", out, errs)
	}
	_, errs, _ = declRun(t, "disown", nil, Diagnostics{DisownNoCurrentJob: "disown: nothing held"})
	if !strings.Contains(errs, "disown: nothing held") {
		t.Errorf("stderr = %q, want the dialect's wording", errs)
	}
}
