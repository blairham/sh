// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Semantics.MonitorAloneAnnouncesAJob, both answers.
//
// The state under test is the one a *script* is in after `set -m`: the monitor
// is running and there is nobody at a prompt to tell. Exactly one column of
// the panel announces a job from there and four say nothing, so the gate
// cannot be Runner.JobControl — which is a prompt and nothing else (#2838).
//
// jobSession with no session is that state: it turns the monitor on with
// `set -m` and leaves JobControl off.
func TestTheMonitorAloneMayAnnounceAJob(t *testing.T) {
	for _, tc := range []struct {
		name     string
		answer   Answer
		announce bool
	}{
		{"the monitor is enough", Yes, true},
		{"the monitor is not enough, which is four of the five", No, false},
		// Read and not asked, for the reason MonitorAloneResumesAJob is: a
		// preset that has not chosen prints nothing rather than putting "the
		// shells disagree here" between a script's commands.
		{"unanswered reads as the quieter answer", Unspecified, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, _ := jobSession(t, &fakeJobs{}, echoCmd+" &\nwait", false,
				func(s *Semantics, _ *Diagnostics) {
					// The dialect announces at all, which is the other axis
					// and is what jobSession switches off as scaffolding.
					s.AnnouncesBackgroundJob = Yes
					s.MonitorAloneAnnouncesAJob = tc.answer
				})
			announced := regexp.MustCompile(`^\[1] [0-9]+\n`).MatchString(out)
			if announced != tc.announce {
				t.Errorf("the script wrote %q; announced=%v, want %v", out, announced, tc.announce)
			}
		})
	}
}

// And having somebody to tell is still enough on its own, in a dialect that
// does not count the monitor.
//
// The two are separate grants and this is the half that would go missing if
// the new one replaced the old: every shell in the panel announces a job at a
// prompt, and only one of them does it from a script.
func TestAPromptStillAnnouncesWithoutTheMonitorAxis(t *testing.T) {
	out, _, _ := jobSession(t, &fakeJobs{}, echoCmd+" &\nwait", true,
		func(s *Semantics, _ *Diagnostics) {
			s.AnnouncesBackgroundJob = Yes
			s.MonitorAloneAnnouncesAJob = No
		})
	if !regexp.MustCompile(`^\[1] [0-9]+\n`).MatchString(out) {
		t.Errorf("the session wrote %q, want the job announced", out)
	}
}

// The state word in a resume notice is the job's own, and `continued` is what
// a job that had to be continued gets.
//
// Measured 2026-09-15 through a pseudo-terminal, zsh 5.9.2, which is the one
// dialect whose notice is a listing row:
//
//	fg on a job stopped by a signal   [1]  + continued  sleep 5
//	fg on a job that is running       [1]  + running    sleep 1
//
// The second is the row `jobs` prints. The wording carried `continued` as a
// literal until the running job was measured, so `fg` on a job that had never
// stopped reported it as continued — a word for something that did not happen
// (#2838).
func TestTheResumeNoticeCarriesTheJobsState(t *testing.T) {
	zshShaped := func(_ *Semantics, d *Diagnostics) {
		d.JobResumedInForeground = "[%[1]d]  %[2]s %-11[4]s%[3]s"
		d.JobContinued = "continued"
		d.JobRunning = "running"
	}
	// A job the shell was told stopped, which `fg` has to continue.
	out, _, _ := jobSession(t, &fakeJobs{waits: []Wait{stopped, {}}},
		echoCmd+"\nfg", true, zshShaped)
	if want := "[1]  + continued  " + echoCmd; lastLine(out) != want {
		t.Errorf("fg on a stopped job said %q, want its last line to be %q", out, want)
	}
	// And one that never stopped, which it only has to put back in front.
	//
	// Held running until `fg` sends the continue, rather than left to finish
	// on its own. `echoCmd` is `/usr/bin/true`, which exits about as fast as
	// a process can, so a `&` job nothing holds open is reaped before `fg`
	// reads its state better than a fifth of the time and the notice says
	// `Done` — a correct word for a job that is genuinely over, and not the
	// one this test exists to grade. The subject of an assertion has to be
	// held still to be measured (#4021).
	out, _, _ = jobSession(t, heldJobs(), echoCmd+" &\nfg", true, zshShaped)
	if want := "[1]  + running    " + echoCmd; lastLine(out) != want {
		t.Errorf("fg on a running job said %q, want its last line to be %q", out, want)
	}
}

// `bg` given a job that is not stopped, which two of the panel refuse.
//
// Measured 2026-09-15 from a script with `set -m` on a pseudo-terminal and
// again at an interactive prompt, the same answers both times:
//
//	bash 5.3.15   bg: job 1 already in background   status 0
//	zsh 5.9.2     bg: job already in background     status 1
//	ksh93u+       the ordinary resume notice        status 0
//	dash          the ordinary resume notice        status 0
//	BusyBox ash   the ordinary resume notice        status 0
//
// The `testsh: ` in front of the two refusals below is this shell's own
// location prefix, which is what carries the builtin's name in zsh — its
// `bg: %1: no such job` is spelled the same way — and is why bash's wording
// names `bg` and zsh's does not.
//
// Two things are asserted of the refusal beyond its text: the resume notice is
// not printed, and the status is the dialect's rather than the 1 a refusal
// usually carries — bash complains and still reports success (#2838).
//
// The job is stopped and resumed once first, so that the second `bg` meets a
// job this shell itself put in the background. That is also the control for
// the note being taken once: a job the script resumed must not read as stopped
// again the next time anything looks.
func TestBgOnAJobThatIsAlreadyRunning(t *testing.T) {
	for _, tc := range []struct {
		name       string
		wording    string
		status     int
		wantLast   string
		wantStatus int
	}{
		{
			name:       "a dialect that does not check resumes it again",
			wantLast:   "[1]+ " + echoCmd + " &",
			wantStatus: 0,
		},
		{
			name:       "the builtin and the number, and success anyway",
			wording:    "%[1]s: job %[2]d already in background",
			status:     0,
			wantLast:   "testsh: bg: job 1 already in background",
			wantStatus: 0,
		},
		{
			name:       "neither named, and a failure",
			wording:    "job already in background",
			status:     1,
			wantLast:   "testsh: job already in background",
			wantStatus: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st, _ := jobSession(t, &fakeJobs{waits: []Wait{stopped}},
				echoCmd+"\nbg\nbg", true, func(_ *Semantics, d *Diagnostics) {
					d.JobResumedInBackground = "[%[1]d]%[2]s %[3]s &"
					d.JobAlreadyInBackground = tc.wording
					d.JobAlreadyInBackgroundStatus = tc.status
				})
			if got := lastLine(out); got != tc.wantLast {
				t.Errorf("the second bg said %q, want its last line to be %q", out, tc.wantLast)
			}
			if st != tc.wantStatus {
				t.Errorf("status %d, want %d", st, tc.wantStatus)
			}
			if tc.wording != "" && strings.Count(out, "&") != 1 {
				t.Errorf("the run wrote %q — the refused bg printed a resume notice", out)
			}
		})
	}
}

// A job stopped from outside the shell is noticed before `fg` decides
// anything, and the two things that turn on it are both asserted here.
//
// `kill -STOP %1` from another line of the script, another terminal or a
// debugger is the shape: the job's own goroutine is told, and until something
// takes that note Job.Stopped still says the job is running. `jobs` takes it
// as part of reaping and `fg` did not, so the same script answered differently
// depending on whether it had a `jobs` line in it — with one, `fg` continued
// the job and waited it out; without, it called a stopped job running and then
// read the stale note in finishResumed and reported 128+SIGSTOP without
// waiting at all (#2838).
//
// The wait is released by the continue rather than by a timer, so the test
// asserts the causal order and does not race it: the second wait returns only
// once `fg` has sent SIGCONT.
func TestFgNoticesAJobStoppedFromOutside(t *testing.T) {
	var once sync.Once
	resumed := make(chan struct{})
	first := make(chan struct{}, 1)
	first <- struct{}{}
	out, st, _ := jobSessionWith(t, &fakeJobs{}, echoCmd+" &\nfg", true, func(r *Runner) {
		dg := *r.Diagnostics
		dg.JobResumedInForeground = "[%[1]d]  %[2]s %-11[4]s%[3]s"
		dg.JobContinued = "continued"
		dg.JobRunning = "running"
		r.Diagnostics = &dg
		r.WaitForCommand = func(int) (Wait, error) {
			select {
			case <-first:
				// Not SIGTSTP: nobody typed anything, which is the whole
				// point of the shape.
				return Wait{Signal: syscall.SIGSTOP, Stopped: true}, nil
			default:
			}
			<-resumed
			return Wait{}, nil
		}
		r.SignalGroup = func(_ int, sig syscall.Signal) error {
			if sig == syscall.SIGCONT {
				once.Do(func() { close(resumed) })
			}
			return nil
		}
	})
	if want := "[1]  + continued  " + echoCmd; lastLine(out) != want {
		t.Errorf("fg said %q, want its last line to be %q — the stop was never taken", out, want)
	}
	if st != 0 {
		t.Errorf("status %d, want 0: fg waited the resumed job out and reported what it left", st)
	}
}
