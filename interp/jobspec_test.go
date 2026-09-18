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

// TestWaitNextJobIsTheFirstToFinish: the first finisher's status, the job
// forgotten, and 127 in silence with nothing to wait for.
func TestWaitNextJobIsTheFirstToFinish(t *testing.T) {
	src := "{ exit 3; } &\nwait -n\necho st=$?\nwait -n\necho again=$?"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		s.WaitNextJob = WaitNextJobFirstToFinish
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

// A job spec that names nothing reports the dialect's number, which is four
// different answers across the panel and is what makes a slot answerable by
// `jobs %n` at all.
//
// Measured 2026-09-05: bash 5.3.15, bash 3.2.57, bash 3.2 as `sh` and ksh93u+
// report 1, dash reports 2, and zsh 5.9.2 reports 127. Zero here means the
// shared 1.
func TestTheStatusForAJobSpecThatNamesNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		dg   Diagnostics
		want string
	}{
		{"the shared answer", Diagnostics{}, "one=1\n"},
		{"a dialect with a number of its own", Diagnostics{NoSuchJobStatus: 2}, "one=2\n"},
		{"and one that calls it a command that is not there", Diagnostics{NoSuchJobStatus: 127}, "one=127\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dg := tc.dg
			out, _ := run(t, `jobs %1 >/dev/null 2>&1; echo "one=$?"`, func(r *Runner) {
				sem := CoreSemantics()
				r.Semantics, r.Diagnostics = &sem, &dg
			})
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The other reading of the same letter, and the four arrangements it takes to
// tell it from the one above and from a plain `wait`. See
// interp.WaitNextJobFirstToSucceed for the measurement behind each (#3245).
func TestWaitNextJobIsTheFirstToSucceed(t *testing.T) {
	succeeds := func(s *Semantics) { s.WaitNextJob = WaitNextJobFirstToSucceed }
	for _, tc := range []struct {
		name string
		src  string
		want string
		why  string
	}{
		{
			"the first job succeeded", "{ /bin/sleep 1; exit 0; } &\n{ /bin/sleep 2; exit 7; } &\nwait -n\necho st=$?",
			"st=0", "a job that exited 0 ends the wait at 0",
		},
		{
			"the first job did not", "{ /bin/sleep 1; exit 7; } &\n{ /bin/sleep 2; exit 0; } &\nwait -n\necho st=$?",
			"st=0", "and the wait goes on until one does, where the other reading stops at 7",
		},
		{
			"no job did", "{ /bin/sleep 1; exit 7; } &\nwait -n\necho st=$?",
			"st=129", "running out without one is a constant, not any job's status",
		},
		{
			"a different failing status", "{ /bin/sleep 1; exit 254; } &\nwait -n\necho st=$?",
			"st=129", "the same constant, which is what says it is not the job's",
		},
		{
			"nothing to wait for", "wait -n\necho st=$?",
			"st=0", "where the other reading answers 127",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, _ := declRun(t, tc.src, succeeds, Diagnostics{})
			if !strings.Contains(out, tc.want) || errs != "" {
				t.Errorf("stdout %q stderr %q, want %q — %s", out, errs, tc.want, tc.why)
			}
		})
	}
	// Operands turn it back into a plain `wait`: both are waited out and the
	// last one's status is reported, which is the half that says the reading
	// above belongs to the no-operand form alone.
	out, _, _ := declRun(t, "{ /bin/sleep 1; exit 7; } &\np=$!\nwait -n $p\necho st=$?", succeeds, Diagnostics{})
	if !strings.Contains(out, "st=7") {
		t.Errorf("stdout = %q, want the job's own status for an operand", out)
	}
}
