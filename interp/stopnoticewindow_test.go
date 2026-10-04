// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"
	"syscall"
	"testing"
	"time"
)

// expectedStopJob is a job in the table with a process, asked to stop, and not
// yet told to have stopped.
//
// Built rather than started, and that is what makes this deterministic. The
// window #4558 is about is a goroutine that has not been scheduled yet, which
// on an idle machine is over before anything can look — the issue says so, and
// says the failure was seen on a loaded CI runner. A note published on a timer
// of the test's own is the same shape with the timing in the test's hands.
func expectedStopJob(r *Runner) *Job {
	j := &Job{
		Command:  "sleep 30",
		polled:   true,
		done:     make(chan struct{}),
		ready:    make(chan struct{}),
		stopNote: make(chan struct{}),
	}
	// A pid the kernel will never have issued, because nothing here signals
	// it: what the field is read for is "this job has a process that could
	// stop", which is the pair Runner.awaitExpectedStops gates on.
	j.settleStartedPID(1<<30, true)
	r.addJob(j)
	r.setLastJob(j)
	return j
}

func leavingRunner(t *testing.T, errs *strings.Builder) *Runner {
	t.Helper()
	sem := CoreSemantics()
	sem.MonitorAloneAccountsForJobsAtExit = Yes
	sem.JobsAtExitSentenceFollowsTheTableOrder = No
	dg := Diagnostics{StoppedJobsAtExit: "STOPPED", RunningJobsAtExit: "RUNNING"}
	r := newTestRunner(t, &Runner{Semantics: &sem, Diagnostics: &dg, Stderr: errs})
	r.monitor = true
	r.SetChecksRunningJobsAtExit(true)
	return r
}

// A stop the script asked for and has not been told about is waited for before
// the shell chooses its sentence (#4558).
//
// `kill -STOP %1` returns when the signal is *sent*, so where that `kill` is a
// script's last command the way out can arrive before the goroutine blocked on
// the process has been told anything. The sweep takes only what that goroutine
// has already said, so the job read as running: the wrong sentence of the two,
// and then a hangup the reference does not send.
//
// The mutant is the call itself — without the wait this row draws RUNNING,
// every time, because the note is a whole scheduling delay away by
// construction.
func TestAStopTheScriptAskedForIsWaitedForOnTheWayOut(t *testing.T) {
	var errs strings.Builder
	r := leavingRunner(t, &errs)
	j := expectedStopJob(r)
	j.stopExpected = true
	go func() {
		time.Sleep(20 * time.Millisecond)
		j.noteStopped(syscall.SIGSTOP)
	}()

	if !r.tellOfJobsLeftBehind(false) {
		t.Fatal("nothing was written about the jobs left behind")
	}
	if got := strings.TrimSpace(errs.String()); got != "STOPPED" {
		t.Errorf("the sentence is %q, want STOPPED: the shell did not wait for the note it was owed", got)
	}
	if !j.Stopped {
		t.Errorf("the job still reads as running, so the hangup would reach a job the reference leaves alone")
	}
}

// And a job nobody asked to stop is not waited for.
//
// The control, and the one that keeps the fix off every other exit: a script
// that stopped nothing has no job carrying the flag, so the way out does not
// reach the select at all. Without this row the wait could have been put on
// every job in the table and the row above would still pass — at the cost of a
// second on the way out of every script with a background job in it.
func TestAJobNobodyStoppedIsNotWaitedForOnTheWayOut(t *testing.T) {
	var errs strings.Builder
	r := leavingRunner(t, &errs)
	expectedStopJob(r)

	start := time.Now()
	if !r.tellOfJobsLeftBehind(false) {
		t.Fatal("nothing was written about the jobs left behind")
	}
	if took := time.Since(start); took > stopNoticeWindow/2 {
		t.Errorf("the way out took %v, want it not to wait for a note nobody is owed", took)
	}
	if got := strings.TrimSpace(errs.String()); got != "RUNNING" {
		t.Errorf("the sentence is %q, want RUNNING", got)
	}
}

// And a note that is never coming costs the window and no more.
//
// SIGSTOP cannot be ignored, so this needs a SIGTSTP aimed at a program that
// has taken the signal for itself — rare, and the reason the wait is bounded
// at all rather than being a plain receive. The row asserts the bound holds
// from both sides: the shell really waited, and it really stopped waiting.
func TestAStopNoteThatNeverComesCostsTheWindowAndNoMore(t *testing.T) {
	var errs strings.Builder
	r := leavingRunner(t, &errs)
	j := expectedStopJob(r)
	j.stopExpected = true

	start := time.Now()
	if !r.tellOfJobsLeftBehind(false) {
		t.Fatal("nothing was written about the jobs left behind")
	}
	took := time.Since(start)
	if took < stopNoticeWindow {
		t.Errorf("the way out took %v, want it to have waited the whole window", took)
	}
	if took > 4*stopNoticeWindow {
		t.Errorf("the way out took %v, want it bounded at %v", took, stopNoticeWindow)
	}
	if got := strings.TrimSpace(errs.String()); got != "RUNNING" {
		t.Errorf("the sentence is %q, want RUNNING: no stop was ever reported", got)
	}
}

// And a `jobs` listing waits for it too, so `kill -STOP $p; jobs %1` lists the
// job stopped, as bash 5.3.20 and ksh93u+ do under the monitor. The note is
// published on the test's own timer, the window shape the exit rows above use;
// without the wait the listing reads `Running` every time.
func TestAListingWaitsForTheStopKillAskedFor(t *testing.T) {
	var out, errs strings.Builder
	r := leavingRunner(t, &errs)
	r.Stdout = &out
	r.Diagnostics.JobStopped = "Stopped"
	r.Diagnostics.JobRunning = "Running"
	j := expectedStopJob(r)
	j.stopExpected = true
	go func() {
		time.Sleep(20 * time.Millisecond)
		j.noteStopped(syscall.SIGSTOP)
	}()

	biJobs(r, t.Context(), nil)
	if !strings.Contains(out.String(), "Stopped") {
		t.Errorf("the listing is %q, want the job stopped: the stop the script asked for was not waited for", out.String())
	}
}
