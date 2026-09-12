// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A job that stopped keeps the current-job marker when a later job joins the
// table, in the five columns that say so — and does not, in the one that
// simply marks the newest.
//
// The arrangement is the whole test: the stopped job has to be the *older*
// one, because "the marker is on the newest job" and "the marker is on the
// stopped job" predict the same thing whenever the newest job is the stopped
// one. Every other listing test here stops all of its jobs, which is exactly
// the case that cannot tell those two apart.
//
// Which shell answers which is asserted in dialect/; this is the mechanism.
func TestAStoppedJobKeepsTheCurrentJobMarker(t *testing.T) {
	for _, tc := range []struct {
		name string
		keep Answer
		// The markers on the older stopped job and the newer running one,
		// and then the jobs `%+` and `%-` resolve to.
		wantStopped, wantNewer string
		wantPlus, wantMinus    string
	}{{
		name:        "the stopped job keeps it",
		keep:        Yes,
		wantStopped: "[1]+", wantNewer: "[2]-",
		wantPlus: "[1]+", wantMinus: "[2]-",
	}, {
		name:        "or the newest job takes it",
		keep:        No,
		wantStopped: "[1]-", wantNewer: "[2]+",
		wantPlus: "[2]+", wantMinus: "[1]-",
	}} {
		t.Run(tc.name, func(t *testing.T) {
			// A foreground command the fake says stopped, and then a real
			// background job started after it. It has to be a job that is
			// still *running*: a `&` that had already finished would be a
			// different state, and the measurement this pins was taken on a
			// running one.
			f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
			keep := tc.keep
			_, _, r := jobRun(t, f, echoCmd+"\n"+lookCmd("sleep")+" 30 &",
				func(s *Semantics) { s.StoppedJobTakesTheCurrentJobMarker = keep })
			jobs := r.Jobs()
			if len(jobs) != 2 {
				t.Fatalf("%d jobs, want the stopped one and the background one", len(jobs))
			}
			// The fake did not really signal anything, so this is the only
			// thing that ends the sleep.
			defer func() { _ = syscall.Kill(-jobs[1].PID, syscall.SIGKILL) }()
			if !jobs[0].Stopped || jobs[1].Stopped {
				t.Fatalf("want job 1 stopped and job 2 running, got %v and %v",
					jobs[0].Stopped, jobs[1].Stopped)
			}

			out, _, _ := jobRun2(t, r, "jobs")
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if len(lines) != 2 {
				t.Fatalf("listed %d jobs, want 2:\n%s", len(lines), out)
			}
			if !strings.HasPrefix(lines[0], tc.wantStopped) {
				t.Errorf("the stopped job is %q, want %q", lines[0], tc.wantStopped)
			}
			if !strings.HasPrefix(lines[1], tc.wantNewer) {
				t.Errorf("the running job is %q, want %q", lines[1], tc.wantNewer)
			}

			// And the specs name the same two jobs the listing marks, which
			// is what makes this more than a column: a bare `fg` follows
			// `%+`, so the two camps resume different jobs after a ^Z.
			plus, _, _ := jobRun2(t, r, "jobs %+")
			if !strings.HasPrefix(strings.TrimSpace(plus), tc.wantPlus) {
				t.Errorf("`jobs %%+` gave %q, want %q", plus, tc.wantPlus)
			}
			minus, _, _ := jobRun2(t, r, "jobs %-")
			if !strings.HasPrefix(strings.TrimSpace(minus), tc.wantMinus) {
				t.Errorf("`jobs %%-` gave %q, want %q", minus, tc.wantMinus)
			}
		})
	}
}

// Stopping a job takes the marker whichever way the axis is answered, which
// is the half that is unanimous: measured with two background jobs and a
// `kill -TSTP` on the older one, every column in the panel moves the `+` onto
// the job it has just stopped. So the axis is about *keeping* the marker and
// not about taking it.
//
// The case that says so is a job stopped a second time, after a job that
// started later than it: the order the two were started in and the order they
// last stopped in disagree, and the marker follows the second.
func TestStoppingAJobTakesTheMarkerEitherWay(t *testing.T) {
	for _, keep := range []Answer{Yes, No} {
		f := &fakeJobs{waits: []Wait{
			{Signal: syscall.SIGTSTP, Stopped: true},
			{Signal: syscall.SIGTSTP, Stopped: true},
			// The `fg %1` below resumes the first and it stops again.
			{Signal: syscall.SIGTSTP, Stopped: true},
		}}
		_, _, r := jobRun(t, f, echoCmd+"\n"+lsCmd,
			func(s *Semantics) { s.StoppedJobTakesTheCurrentJobMarker = keep })
		if _, _, _ = jobRun2(t, r, "fg %1"); len(r.Jobs()) != 2 {
			t.Fatalf("keep=%v: %d jobs after `fg %%1`, want 2", keep, len(r.Jobs()))
		}
		out, _, _ := jobRun2(t, r, "jobs")
		lines := strings.Split(strings.TrimSpace(out), "\n")
		if len(lines) != 2 {
			t.Fatalf("keep=%v: listed %d jobs, want 2:\n%s", keep, len(lines), out)
		}
		if !strings.HasPrefix(lines[0], "[1]+") || !strings.HasPrefix(lines[1], "[2]-") {
			t.Errorf("keep=%v: listing is %q, want the re-stopped job marked `+`", keep, out)
		}
	}
}
