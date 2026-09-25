// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
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
//
// The login condition is a dialect answer since #4509 — bash requires one and
// zsh does not — so this helper takes bash's, and the suite that is *about*
// the axis sets it itself.
func hupJobs(t *testing.T, interactive, login, option bool) (*fakeJobs, *Runner) {
	t.Helper()
	return hupJobsShaped(t, interactive, login, option, func(s *Semantics, _ *Diagnostics) {
		s.HangupAtExitNeedsALoginShell = Yes
	})
}

func hupJobsShaped(t *testing.T, interactive, login, option bool,
	shape func(*Semantics, *Diagnostics),
) (*fakeJobs, *Runner) {
	t.Helper()
	f := heldJobs()
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = No
	dg := Diagnostics{}
	shape(&sem, &dg)
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

// TestTheJobsAreHungUpOnlyForAnInteractiveLoginShell is the four rows, in the
// dialect that asks for a login shell. The three negatives are the substance:
// with the option alone the shell would hang up every job of every `sh -c` in
// a pipeline.
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

// The login condition is the dialect's, not the engine's — the two shells
// that hang up at all disagree about it.
//
// bash needs one (the rows above). zsh does not: measured 2026-09-25 through
// a pseudo-terminal with the shell started `-fiV +Z`, which is interactive
// and not a login shell, `setopt no_check_jobs` then `sleep 30 &` then `exit`
// writes `zsh: warning: 1 jobs SIGHUPed` and the job is gone half a second
// later.
//
// Both answers in one table, because the axis is the subject: a test that
// only asked the Yes half would pass for an engine that had the condition
// nailed in, which is what it was until #4509.
func TestWhetherTheHangupNeedsALoginShellIsTheDialects(t *testing.T) {
	for _, tc := range []struct {
		why   string
		axis  Answer
		login bool
		want  int
	}{
		{"asked for, and a login shell", Yes, true, 1},
		{"asked for, and not a login shell", Yes, false, 0},
		{"not asked for, and a login shell", No, true, 1},
		{"not asked for, and not a login shell", No, false, 1},
	} {
		t.Run(tc.why, func(t *testing.T) {
			f, r := hupJobsShaped(t, true, tc.login, true, func(s *Semantics, _ *Diagnostics) {
				s.HangupAtExitNeedsALoginShell = tc.axis
			})
			jobRun2(t, r, "/usr/bin/true &")
			r.Finish(t.Context())
			if got := hangups(f); got != tc.want {
				t.Errorf("%d hangups, want %d", got, tc.want)
			}
		})
	}
}

// A stopped job is left out of the send where the dialect says so, and is in
// it where the dialect does not.
//
// zsh Yes: measured 2026-09-25 through a pseudo-terminal, a session holding
// one running job and one `kill -STOP`ped one says `1 jobs SIGHUPed`, and a
// session holding two stopped jobs and nothing running says nothing at all.
// bash No, which is what its implementation already did — see the axis for
// why that row is not a measurement and what stands in the way of one.
//
// The running job in each row is the control: it says the send happened, so a
// stopped job missing from the count is the rule rather than a hangup that
// did not run.
func TestWhetherAStoppedJobIsHungUpIsTheDialects(t *testing.T) {
	for _, tc := range []struct {
		why  string
		axis Answer
		want int
	}{
		{"skipped", Yes, 1},
		{"included", No, 2},
	} {
		t.Run(tc.why, func(t *testing.T) {
			f, r := hupJobsShaped(t, true, true, true, func(s *Semantics, _ *Diagnostics) {
				s.HangupAtExitSkipsStoppedJobs = tc.axis
			})
			// The stopped one first: the fake's canned answer is consumed by
			// the foreground command, and a `&` job of the same shape would
			// be told it had stopped before it had started.
			f.waits = []Wait{stopped}
			jobRun2(t, r, echoCmd)
			jobRun2(t, r, "/usr/bin/true &")
			jobs := r.Jobs()
			if len(jobs) != 2 || !jobs[0].Stopped || jobs[1].Stopped {
				t.Fatalf("jobs = %+v, want one stopped and one running", jobs)
			}
			r.Finish(t.Context())
			if got := hangups(f); got != tc.want {
				t.Errorf("%d hangups, want %d", got, tc.want)
			}
		})
	}
}

// And the sentence, which is the count of what was signaled rather than of
// what was in the table.
//
// Measured 2026-09-25 against zsh 5.9.2 through a pseudo-terminal: one
// running job and one stopped is `zsh: warning: 1 jobs SIGHUPed`, three
// running is `3 jobs SIGHUPed`, and a session with nothing to send to says
// nothing. `jobs` however many, including one.
//
// The empty-wording row is the other four columns: bash has the capability
// and says nothing about it, so the send has to be separable from the
// sentence.
func TestTheHangupSaysHowManyJobsItReached(t *testing.T) {
	const wording = "%[1]s: warning: %[2]d jobs SIGHUPed"
	for _, tc := range []struct {
		why     string
		wording string
		stopped bool
		jobs    int
		want    string
	}{
		{"one running job", wording, false, 1, "testsh: warning: 1 jobs SIGHUPed"},
		{"three running jobs", wording, false, 3, "testsh: warning: 3 jobs SIGHUPed"},
		{"a stopped one beside it is outside the count", wording, true, 1, "testsh: warning: 1 jobs SIGHUPed"},
		{"nothing to send to", wording, false, 0, ""},
		{"a stopped job alone", wording, true, 0, ""},
		{"a dialect with no words for it", "", false, 1, ""},
	} {
		t.Run(tc.why, func(t *testing.T) {
			out := sink(t)
			f, r := hupJobsShaped(t, true, true, true, func(s *Semantics, d *Diagnostics) {
				s.HangupAtExitSkipsStoppedJobs = Yes
				d.JobsHUPedAtExit = tc.wording
			})
			r.Stdout, r.Stderr = out, out
			if tc.stopped {
				f.waits = []Wait{stopped}
				jobRun2(t, r, echoCmd)
			}
			for range tc.jobs {
				jobRun2(t, r, "/usr/bin/true &")
			}
			r.Finish(t.Context())
			got := strings.TrimSpace(read(t, out))
			if got != tc.want {
				t.Errorf("said %q, want %q", got, tc.want)
			}
		})
	}
}

// Which side of the EXIT trap it falls on is the dialect's too.
//
// bash No: measured 2026-09-23, the trap's line arrives and *then* the job's
// HUP handler runs. zsh Yes: measured 2026-09-25 through a pseudo-terminal
// with `trap 'echo EXIT_TRAP' EXIT` and `sleep 3 &` in an interactive
// session, `exit` writes `zsh: warning: 1 jobs SIGHUPed` and then
// `EXIT_TRAP`.
//
// Read off the order of two lines on one stream, which is why both halves
// write: a test that only checked that the warning appeared would pass for
// either answer.
func TestWhichSideOfTheExitTrapTheHangupFallsOnIsTheDialects(t *testing.T) {
	for _, tc := range []struct {
		why  string
		axis Answer
		want []string
	}{
		{"before the trap", Yes, []string{"testsh: warning: 1 jobs SIGHUPed", "EXIT_TRAP"}},
		{"after the trap", No, []string{"EXIT_TRAP", "testsh: warning: 1 jobs SIGHUPed"}},
	} {
		t.Run(tc.why, func(t *testing.T) {
			out := sink(t)
			_, r := hupJobsShaped(t, true, true, true, func(s *Semantics, d *Diagnostics) {
				s.HangupAtExitPrecedesTheExitTrap = tc.axis
				d.JobsHUPedAtExit = "%[1]s: warning: %[2]d jobs SIGHUPed"
			})
			r.Stdout, r.Stderr = out, out
			jobRun2(t, r, "/usr/bin/true &")
			jobRun2(t, r, "trap 'echo EXIT_TRAP' EXIT")
			r.Finish(t.Context())
			lines := strings.Split(strings.TrimSpace(read(t, out)), "\n")
			if len(lines) != 2 || lines[0] != tc.want[0] || lines[1] != tc.want[1] {
				t.Errorf("wrote %q, want %q", lines, tc.want)
			}
		})
	}
}
