// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A shell tells whoever is typing about its jobs, twice: when one is
// backgrounded, and when it ends. Neither happens to a script — no shell in
// the panel announces anything to `sh -c` — so both hang off the front end
// having said there is someone to tell.
func TestABackgroundedJobIsAnnounced(t *testing.T) {
	for _, tc := range []struct {
		name       string
		jobControl bool
		announces  Answer
		dg         Diagnostics
		want       string
	}{
		{"announced at a prompt", true, Yes, Diagnostics{}, "[1] "},
		{"and the separator is the dialect's", true, Yes, Diagnostics{JobStarted: "[%[1]d]\t%[2]d"}, "[1]\t"},
		{"a dialect can say nothing", true, No, Diagnostics{}, ""},
		// The one that matters most: a script is told nothing, whatever the
		// dialect would do at a prompt.
		{"and a script is told nothing", false, Yes, Diagnostics{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := notices(t, `sleep 0.3 &`, tc.jobControl, tc.announces, tc.dg, nil)
			if tc.want == "" {
				if strings.Contains(out, "[1]") {
					t.Errorf("out = %q, want nothing announced", out)
				}
				return
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("out = %q, want it to contain %q", out, tc.want)
			}
		})
	}
}

// The notice is taken rather than read, and reported once: the same rule a
// listing follows, because saying it twice is exactly what a shell must not
// do.
func TestAFinishedJobIsReportedOnceAndForgotten(t *testing.T) {
	var r *Runner
	out := notices(t, `true & sleep 0.05`, true, No, Diagnostics{}, &r)
	if strings.Contains(out, "Done") {
		t.Fatalf("out = %q, want nothing said before the notice is asked for", out)
	}
	first := r.FinishedJobNotices()
	if len(first) != 1 || !strings.Contains(first[0], "Done") {
		t.Fatalf("notices = %v, want the finished job reported", first)
	}
	if again := r.FinishedJobNotices(); len(again) != 0 {
		t.Errorf("notices = %v the second time, want it reported once", again)
	}
	// And gone, so a listing does not repeat what the notice already said.
	if n := len(r.Jobs()); n != 0 {
		t.Errorf("%d jobs left, want the reported one forgotten", n)
	}
}

// A shell with nobody to tell says nothing and keeps the job, because a
// script's `jobs` still has to be able to list it.
func TestWithoutJobControlNothingIsReported(t *testing.T) {
	var r *Runner
	notices(t, `true & sleep 0.05`, false, Yes, Diagnostics{}, &r)
	if lines := r.FinishedJobNotices(); len(lines) != 0 {
		t.Errorf("notices = %v, want none without someone to tell", lines)
	}
	if n := len(r.Jobs()); n != 1 {
		t.Errorf("%d jobs, want the job kept for a listing to find", n)
	}
}

// The command is in the notice even in the dialects that leave it out of a
// listing — both of them print it here, which is what makes
// JobsShowBackgroundCommand a question about the listing and not about the
// text that was kept.
func TestANoticeShowsTheCommandAListingWouldNot(t *testing.T) {
	var r *Runner
	notices(t, `true & sleep 0.05`, true, No, Diagnostics{JobUnknownCommand: "<command unknown>"}, &r)
	lines := r.FinishedJobNotices()
	if len(lines) != 1 || !strings.Contains(lines[0], "true") {
		t.Errorf("notices = %v, want the command in the notice", lines)
	}
}

// One dialect puts the `&` back when it reports that a job ended, and it is
// not the one that puts it back while a job runs. Neither does both.
func TestANoticeCanCarryTheAmpersand(t *testing.T) {
	var r *Runner
	notices(t, `true & sleep 0.05`, true, No, Diagnostics{JobNoticeShowsAmpersand: true}, &r)
	lines := r.FinishedJobNotices()
	if len(lines) != 1 || !strings.HasSuffix(strings.TrimRight(lines[0], " "), "true &") {
		t.Errorf("notices = %v, want the command with its ampersand", lines)
	}
}

// One dialect says `Done` in a notice and lists the same job as `Running`,
// because its listing has not noticed what its reaper already said. Two
// statements about one job, and both are that shell's.
func TestANoticeCanUseADifferentWordFromAListing(t *testing.T) {
	var r *Runner
	dg := Diagnostics{JobDone: "Running", JobDoneNotice: "Done"}
	notices(t, `true & sleep 0.05`, true, No, dg, &r)
	lines := r.FinishedJobNotices()
	if len(lines) != 1 || !strings.Contains(lines[0], "Done") {
		t.Errorf("notices = %v, want the notice's own word", lines)
	}
}

func notices(t *testing.T, src string, jobControl bool, announces Answer, dg Diagnostics, into **Runner) string {
	t.Helper()
	out, st := run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.AnnouncesBackgroundJob = announces
		sem.JobsShowBackgroundCommand = Yes
		sem.JobsListFinishedJobs = Yes
		sem.JobsListNewestFirst = No
		r.Semantics, r.Diagnostics = &sem, &dg
		r.JobControl = jobControl
		if into != nil {
			*into = r
		}
	})
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	return out
}

// `$!` survives the job it names — the job ending, the notice reporting it,
// and the table forgetting it.
//
// Measured unanimous 2026-09-05: `sleep 0 & wait; echo "[$!]"` reports the pid
// in bash 5.3.15, bash 3.2.57, bash 3.2 run as `sh`, dash, ksh93u+ and zsh
// 5.9.2, and so does the same script under `-i script.sh` on a pseudo-terminal
// where the `Done` row has already been printed.
//
// It did not, and the reason is the shape rather than the rule: `$!` read the
// *current job*, which the notice has to drop because `%%` and a bare `fg`
// would otherwise name a job that is no longer in the table. Two questions,
// so two fields.
func TestTheLastBackgroundPidSurvivesTheNotice(t *testing.T) {
	var r *Runner
	out := notices(t, `true & sleep 0.05`, true, No, Diagnostics{}, &r)
	if st := strings.TrimSpace(out); st != "" {
		t.Fatalf("out = %q, want nothing before the notice is asked for", out)
	}
	before := bang(t, r)
	if before == "" {
		t.Fatal("`$!` was empty before the notice — the job was never recorded")
	}
	if lines := r.FinishedJobNotices(); len(lines) != 1 {
		t.Fatalf("notices = %v, want the finished job reported once", lines)
	}
	if n := len(r.Jobs()); n != 0 {
		t.Fatalf("%d jobs left, want the reported one forgotten", n)
	}
	if after := bang(t, r); after != before {
		t.Errorf("`$!` = %q after the notice, want %q — the pid outlives the job", after, before)
	}
}

// And before any background command there is nothing to report.
//
// Five of the six: `echo "[$!]"` writes `[]` in bash 5.3.15, bash 3.2.57,
// bash 3.2 run as `sh`, dash and ksh93u+. zsh 5.9.2 writes `[0]`, and is the
// only shell in the panel that answers with a number nothing ever had. The
// core takes the reading that invents nothing.
//
// Zero is not the same answer as nothing here, which is why whether a job has
// been started is its own fact: a background builtin runs in this process and
// its job carries no pid, so a runner really can hold a recorded zero.
func TestTheLastBackgroundPidIsEmptyBeforeAnyJob(t *testing.T) {
	var r *Runner
	notices(t, `:`, true, No, Diagnostics{}, &r)
	if got := bang(t, r); got != "" {
		t.Errorf("`$!` = %q with no background command started, want it empty", got)
	}
}

// bang is what this runner expands `$!` to, read the way a script reads it.
func bang(t *testing.T, r *Runner) string {
	t.Helper()
	return r.Expand("$!")
}

// The jobs a caller was handed stay the jobs it was handed.
//
// Reporting a finished job compacts the table in place and clears the slots
// past the survivors, so that a job nobody can list is not held alive by an
// array nobody reads. A caller still holding the longer header it took a
// moment earlier would find a nil where its job had been, and `Wait` on a nil
// job is a segmentation fault rather than an error.
//
// A prompt drawn between taking the slice and walking it is the whole of what
// it takes, which is exactly what a shell does between one line and the next.
func TestTheSliceOfJobsSurvivesTheTableBeingReaped(t *testing.T) {
	var r *Runner
	notices(t, `true & sleep 0.05`, true, No, Diagnostics{}, &r)
	taken := r.Jobs()
	if len(taken) != 1 {
		t.Fatalf("%d jobs, want the one that was started", len(taken))
	}
	held := taken[0]
	if lines := r.FinishedJobNotices(); len(lines) != 1 {
		t.Fatalf("notices = %v, want the finished job reported once", lines)
	}
	if n := len(r.Jobs()); n != 0 {
		t.Fatalf("%d jobs left, want the reported one forgotten", n)
	}
	if taken[0] != held {
		t.Fatalf("the caller's slice holds %v after the reap, want the job it was handed", taken[0])
	}
	// And what it holds is still answerable, which is why a caller holds a job
	// at all: it took the job while it was running and wants its status now.
	if st := taken[0].Wait(); st != 0 {
		t.Errorf("the job's status is %d, want 0", st)
	}
}
