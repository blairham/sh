// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A shell can be asked to send SIGHUP to the jobs it still has when it ends,
// and one dialect calls that `shopt -s huponexit`.
//
// Measured 2026-09-23 on bash 5.3.15 through a pty, a `sleep` left in the
// background and counted afterwards:
//
//	-l -i, option on      the job is gone
//	-l -i, option off     the job survives
//	-i, not login, on     the job survives
//	-l ./s.sh, on         the job survives
//
// So **both** conditions are required, interactive and login, and the EXIT
// trap runs before the signal.
//
// Two instruments were wrong before this one. A backgrounded subshell holding
// a `HUP` trap works against bash and **cannot** work here — `( … ) &` is a
// cloned runner in this process rather than a forked shell, so the signal
// reaches the `sleep` it started and there is no shell in between to trap it.
// That instrument reported the option doing nothing while it was working, and
// an instrument that is asymmetric between the reference and the subject is
// worse than one that is merely blunt. Counting survivors is symmetric, which
// is what the rows above are.
//
// Here the group signal is recorded rather than sent, for the reason the rest
// of jobcontrol_test.go records it: the decision is what this file is about,
// and a test that asked the kernel would be grading the kernel.

// hupJobs is a runner at a prompt whose jobs keep running, with the two
// invocation facts the option needs set as the caller asks.
func hupJobs(t *testing.T, interactive, login, option bool) (*fakeJobs, *Runner) {
	t.Helper()
	f := heldJobs()
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = No
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &dg, Name: "testsh", JobControl: true,
	})
	r.Interactive, r.LoginShell = interactive, login
	r.SetSendsHangupToJobsAtExit(option)
	r.Terminal = true
	if code := r.SetOptionLetters("m", true); code != 0 {
		t.Fatalf("set -m: status %d", code)
	}
	r.WaitForCommand = f.waitFor
	r.SignalGroup = func(pgid int, sig syscall.Signal) error {
		f.signals = append(f.signals, struct {
			pgid int
			sig  syscall.Signal
		}{pgid, sig})
		if sig == syscall.SIGCONT {
			f.releaseHeld()
		}
		return nil
	}
	r.PollCommand = func(int) (Wait, bool, error) {
		w, changed := f.poll()
		return w, changed, nil
	}
	t.Cleanup(func() { f.releaseHeld(); f.reapSaidStopped(t) })
	return f, r
}

// hangups counts the SIGHUPs the fake was asked to send.
func hangups(f *fakeJobs) int {
	n := 0
	for _, s := range f.signals {
		if s.sig == syscall.SIGHUP {
			n++
		}
	}
	return n
}

// TestTheJobsAreHungUpOnlyForAnInteractiveLoginShell is the four rows. The
// three negatives are the substance: with the option alone the shell would
// hang up every job of every `sh -c` in a pipeline.
func TestTheJobsAreHungUpOnlyForAnInteractiveLoginShell(t *testing.T) {
	for _, tc := range []struct {
		why                        string
		interactive, login, option bool
		want                       int
	}{
		{"interactive login shell with the option", true, true, true, 1},
		{"the same without the option", true, true, false, 0},
		{"interactive but not a login shell", true, false, true, 0},
		{"a login shell that is not interactive", false, true, true, 0},
		{"neither, with the option", false, false, true, 0},
	} {
		t.Run(tc.why, func(t *testing.T) {
			f, r := hupJobs(t, tc.interactive, tc.login, tc.option)
			jobRun2(t, r, "/usr/bin/true &")
			// Finish explicitly: jobRun2 uses RunPart, which is a shell that
			// has not ended, and the hangup is a thing a shell does *as* it
			// ends. Without this every row reads 0 and the test says the
			// option never works.
			r.Finish(t.Context())
			if got := hangups(f); got != tc.want {
				t.Errorf("%d hangups, want %d", got, tc.want)
			}
		})
	}
}

// TestAHangupGoesToTheJobsGroup: it is the job's process group that is
// signaled, not one process, which is what makes a job with a pipeline in it
// die whole.
func TestAHangupGoesToTheJobsGroup(t *testing.T) {
	f, r := hupJobs(t, true, true, true)
	_, _, r = jobRun2(t, r, "/usr/bin/true &")
	jobs := r.Jobs()
	r.Finish(t.Context())
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want one", len(jobs))
	}
	var sent []int
	for _, s := range f.signals {
		if s.sig == syscall.SIGHUP {
			sent = append(sent, s.pgid)
		}
	}
	if len(sent) != 1 || sent[0] != jobs[0].PID {
		t.Errorf("hung up groups %v, want the job's own %d", sent, jobs[0].PID)
	}
}
