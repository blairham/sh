// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.MonitorAloneResumesAJob, both answers.
//
// The state under test is the one a *script* is in after `set -m`: the
// monitor is running and there is nobody to announce jobs to. Four of the
// five columns run a job from there and one refuses, so the gate cannot be
// Runner.JobControl — which is a prompt and nothing else — and it cannot be
// the terminal either, since bash resumes without one (#2720).
//
// jobSession with no session is exactly that state: it turns the monitor on
// with `set -m` and leaves JobControl off, which is what the comment in its
// own body says it is for.
func TestTheMonitorAloneMayResumeAJob(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		wantOut string
		wantSt  int
	}{
		// The job is named on the way in and run, and the status is the
		// job's rather than a refusal's.
		{"the monitor is enough", Yes, echoCmd + "\n", 0},
		// And the shell that wants more than the monitor refuses exactly as
		// it did before the axis existed — the command is never named,
		// which is the half #2657 is about.
		{"the monitor is not enough", No, "testsh: fg: no job control\n", 1},
		// Unanswered reads as the stricter answer rather than complaining:
		// this is read and not asked, so a preset that has not chosen
		// refuses the resume instead of putting "the shells disagree here"
		// in the middle of a script's job handling.
		{"unanswered reads as the stricter answer", Unspecified, "testsh: fg: no job control\n", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped, {}}}
			out, st, _ := jobSession(t, f, echoCmd+"\nfg", false, func(s *Semantics, d *Diagnostics) {
				s.MonitorAloneResumesAJob = tc.answer
				s.JobControlAbsenceIsReportedFirst = Yes
				d.NoJobControl = "%[1]s: no job control"
			})
			if out != tc.wantOut {
				t.Errorf("the run wrote %q, want %q", out, tc.wantOut)
			}
			if st != tc.wantSt {
				t.Errorf("status %d, want %d", st, tc.wantSt)
			}
		})
	}
}

// And the monitor is the gate rather than a second name for it: with the
// monitor off — which is every script that did not ask for it — the answer
// that resumes still refuses, in the shell that resumes on the monitor alone.
//
// Measured: the same script without `set -m` is refused in every column, on a
// pseudo-terminal and off one.
func TestWithNoMonitorTheResumeIsStillRefused(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped, {}}}
	out, st, _ := jobSessionWith(t, f, echoCmd+"\nfg", false, func(r *Runner) {
		sem := *r.Semantics
		sem.MonitorAloneResumesAJob = Yes
		sem.JobControlAbsenceIsReportedFirst = Yes
		r.Semantics = &sem
		dg := *r.Diagnostics
		dg.NoJobControl = "%[1]s: no job control"
		r.Diagnostics = &dg
		if code := r.SetOptionLetters("m", false); code != 0 {
			t.Fatalf("set +m: status %d", code)
		}
	})
	if !strings.Contains(out, "no job control") {
		t.Errorf("the run wrote %q, want the refusal", out)
	}
	if strings.Contains(out, echoCmd) {
		t.Errorf("the run wrote %q, which names the job — a refusal prints no command", out)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
}

// TestFgWaitsOutABackgroundJobSomebodyElseHolds is the other half of the same
// repro, and it is a different fault: once the gate opened, `fg` reached a
// job whose process this shell is not the waiter for.
//
// A `&` job has a goroutine of its own blocked on it, so the front end's wait
// hook — which is a `waitpid` — answers ECHILD when `fg` calls it for the
// same pid. The shell named the job on stdout and then wrote `fg: no child
// processes` at 1, where the panel runs the job and reports its status. Before
// `set -m` reached the builtin the only resumable job was one ^Z had left
// behind, which nobody is waiting on, so the shape was unreachable (#2720).
func TestFgWaitsOutABackgroundJobSomebodyElseHolds(t *testing.T) {
	f := &fakeJobs{}
	out, st, _ := jobSessionWith(t, f, echoCmd+" &\nfg", false, func(r *Runner) {
		sem := *r.Semantics
		sem.MonitorAloneResumesAJob = Yes
		r.Semantics = &sem
		// The hook answers the way a real `waitpid` answers a caller that
		// does not hold the child, which is the whole of the fault: the
		// fake's canned reply would let a second wait succeed and the row
		// would pass against the code it is here to catch.
		r.WaitForCommand = func(int) (Wait, error) { return Wait{}, syscall.ECHILD }
	})
	if out != echoCmd+"\n" {
		t.Errorf("the run wrote %q, want the job named and nothing else", out)
	}
	if strings.Contains(out, "child") {
		t.Errorf("the run wrote %q — `fg` waited for a process it does not hold", out)
	}
	if st != 0 {
		t.Errorf("status %d, want the job's own 0", st)
	}
}
