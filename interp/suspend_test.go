// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"syscall"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// jobSession is jobRun with the two things a stop notice depends on and which
// jobRun deliberately has neither of: somebody to tell, and a dialect that has
// said how to word it.
//
// The output is one file for both streams, so the order between a notice and
// what the resumed command printed is the order they were written — the
// notices are the only thing these tests run anything for.
func jobSession(t *testing.T, f *fakeJobs, src string, jobControl bool, shape func(*Semantics, *Diagnostics)) (string, int, *Runner) {
	t.Helper()
	return jobSessionShaped(t, f, src, jobControl, shape, nil)
}

// jobSessionWith is the same with the dialect left alone and the Runner handed
// to the caller before it runs, for a test that wires a front-end hook.
func jobSessionWith(t *testing.T, f *fakeJobs, src string, jobControl bool, adjust func(*Runner)) (string, int, *Runner) {
	t.Helper()
	return jobSessionShaped(t, f, src, jobControl, func(*Semantics, *Diagnostics) {}, adjust)
}

func jobSessionShaped(t *testing.T, f *fakeJobs, src string, jobControl bool, shape func(*Semantics, *Diagnostics), adjust func(*Runner)) (string, int, *Runner) {
	t.Helper()
	file, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out := sink(t)
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = No
	sem.JobsListNewestFirst = No
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	sem.StoppedJobsHoldTheExit = Yes
	// A `&` job in one of these is scaffolding rather than the subject, so the
	// announcement is answered here and not left to complain.
	sem.AnnouncesBackgroundJob = No
	dg := Diagnostics{}
	shape(&sem, &dg)
	r := newTestRunner(t, &Runner{
		Stdout: out, Stderr: out, Semantics: &sem, Diagnostics: &dg,
		// Interactive alongside JobControl, which is how a prompt sets them:
		// one says there is somebody to tell about a job and the other says
		// the shell is a session. What an interrupt does to the line is asked
		// of the second.
		Name: "testsh", JobControl: jobControl, Interactive: jobControl,
	})
	r.WaitForCommand = func(int) (Wait, error) { return f.next(), nil }
	r.SignalGroup = func(int, syscall.Signal) error { return nil }
	r.Foreground = func(pgid int) error {
		f.foreground = append(f.foreground, pgid)
		return nil
	}
	r.PollCommand = func(int) (Wait, bool, error) {
		w, changed := f.poll()
		return w, changed, nil
	}
	if adjust != nil {
		adjust(r)
	}
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return read(t, out), r.ExitStatus(), r
}

// stopped is the wait a ^Z produces.
var stopped = Wait{Signal: syscall.SIGTSTP, Stopped: true}

// What ^Z leaves on the screen.
//
// A shell with somebody to tell says a job stopped, and says it *there* rather
// than before the next prompt: measured through a pseudo-terminal, `sleep 5;
// echo after` stopped with ^Z prints the notice and then `after`, in every
// shell in the panel. A shell with nobody to tell — a script — says nothing,
// which is the rule a backgrounded job's announcement already follows.
func TestAStoppedForegroundJobIsAnnouncedToAPerson(t *testing.T) {
	for _, tc := range []struct {
		name       string
		jobControl bool
		want       bool
	}{
		{"a prompt is told", true, true},
		{"a script is not", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			out, _, _ := jobSession(t, f, echoCmd, tc.jobControl, func(*Semantics, *Diagnostics) {})
			if got := strings.Contains(out, "Stopped"); got != tc.want {
				t.Errorf("announced = %v, want %v (said %q)", got, tc.want, out)
			}
		})
	}
}

// The notice is the listing's own row unless the dialect words it otherwise,
// and it starts on a line of its own only where the dialect says so.
//
// Both halves are measured through a pseudo-terminal, which is the only place
// either exists: bash writes a newline and then `[1]+  Stopped   sleep 40`,
// dash and ksh93 write their own row straight after the `^Z` the terminal
// echoed, and zsh writes a sentence that names itself and no job number at all.
func TestTheStoppedNoticeIsTheDialectsOwn(t *testing.T) {
	for _, tc := range []struct {
		name  string
		shape func(*Semantics, *Diagnostics)
		want  string
	}{
		{
			name:  "a listing row, straight after the echo",
			shape: func(*Semantics, *Diagnostics) {},
			want:  "[1]+  Stopped",
		},
		{
			name:  "the same row, on a line of its own",
			shape: func(_ *Semantics, d *Diagnostics) { d.JobStoppedNoticeOnANewLine = true },
			want:  "\n[1]+  Stopped",
		},
		{
			name:  "a sentence naming the shell",
			shape: func(_ *Semantics, d *Diagnostics) { d.JobStoppedNotice = "%[3]s: suspended  %[4]s" },
			want:  "testsh: suspended  " + echoCmd + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			out, _, _ := jobSession(t, f, echoCmd, true, tc.shape)
			if !strings.HasPrefix(out, tc.want) {
				t.Errorf("notice = %q, want it to begin %q", out, tc.want)
			}
			if !strings.HasSuffix(out, echoCmd+"\n") {
				t.Errorf("notice = %q, want it to name the command and end a line", out)
			}
		})
	}
}

// A job that stops again while `fg` is waiting for it says so again, is the
// current job again, and stays in the table.
func TestAJobThatStopsUnderFgSaysSoAgain(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped, stopped}}
	out, _, r := jobSession(t, f, echoCmd+"\nfg", true, func(*Semantics, *Diagnostics) {})
	if n := strings.Count(out, "Stopped"); n != 2 {
		t.Errorf("said %q, want the stop reported both times", out)
	}
	jobs := r.Jobs()
	if len(jobs) != 1 || !jobs[0].Stopped {
		t.Fatalf("jobs = %+v, want the one job still stopped", jobs)
	}
	if got := jobs[0].StopSig; got != int(syscall.SIGTSTP) {
		t.Errorf("StopSig = %d, want %d", got, syscall.SIGTSTP)
	}
}

// `fg` and `bg` name the job the dialect's way.
//
// Measured: bash's `fg` prints the command alone and its `bg` prints `[1]+
// sleep 40 &`; zsh prints the same listing row for both, with a state —
// `continued` — that appears in no listing. Run without a person to tell, so
// that what the resume printed is the whole of the output.
func TestTheResumeNoticeIsTheDialectsOwn(t *testing.T) {
	for _, tc := range []struct {
		name           string
		shape          func(*Semantics, *Diagnostics)
		wantFg, wantBg string
	}{
		{
			name:   "nothing said, so the command, and the ampersand for bg",
			shape:  func(*Semantics, *Diagnostics) {},
			wantFg: echoCmd + "\n",
			wantBg: echoCmd + " &\n",
		},
		{
			name: "the row's head, and the ampersand after the command",
			shape: func(_ *Semantics, d *Diagnostics) {
				d.JobResumedInBackground = "[%[1]d]%[2]s %[3]s &"
			},
			wantFg: echoCmd + "\n",
			wantBg: "[1]+ " + echoCmd + " &\n",
		},
		{
			name: "a row with a state of its own, for both",
			shape: func(_ *Semantics, d *Diagnostics) {
				d.JobResumedInForeground = "[%[1]d]  %[2]s continued  %[3]s"
				d.JobResumedInBackground = "[%[1]d]  %[2]s continued  %[3]s"
			},
			wantFg: "[1]  + continued  " + echoCmd + "\n",
			wantBg: "[1]  + continued  " + echoCmd + "\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// `fg` waits, so the second wait is the resumed command ending.
			fg, _, _ := jobSession(t, &fakeJobs{waits: []Wait{stopped, {Status: 0}}},
				echoCmd+"\nfg", false, tc.shape)
			if fg != tc.wantFg {
				t.Errorf("fg said %q, want %q", fg, tc.wantFg)
			}
			bg, _, _ := jobSession(t, &fakeJobs{waits: []Wait{stopped}},
				echoCmd+"\nbg", false, tc.shape)
			if bg != tc.wantBg {
				t.Errorf("bg said %q, want %q", bg, tc.wantBg)
			}
		})
	}
}

// A job `bg` let go of is finished by the shell asking after it, and that is
// the only thing that can finish one: nothing is waiting on it — the wait that
// returned when it stopped is over — so without the ask it stays listed as
// running for the rest of the session.
func TestAResumedJobIsFinishedByAsking(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}, polls: []Wait{{Status: 0}}}
	_, _, r := jobSession(t, f, echoCmd+"\nbg", true, func(*Semantics, *Diagnostics) {})
	if jobs := r.Jobs(); len(jobs) != 1 || jobs[0].Finished() {
		t.Fatalf("jobs = %+v, want one that has not finished yet", jobs)
	}
	notices := r.FinishedJobNotices()
	if len(notices) != 1 || !strings.Contains(notices[0], "Done") {
		t.Errorf("notices = %q, want the job reported as done", notices)
	}
	if len(r.Jobs()) != 0 {
		t.Error("the job was reported and not forgotten")
	}
}

// And a job that has not changed state is left exactly as it was, which is
// what keeps a running one from being reported as finished.
func TestAskingAboutAJobThatIsStillRunningChangesNothing(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}}
	_, _, r := jobSession(t, f, echoCmd+"\nbg", true, func(*Semantics, *Diagnostics) {})
	if notices := r.FinishedJobNotices(); len(notices) != 0 {
		t.Errorf("notices = %q, want none", notices)
	}
	jobs := r.Jobs()
	if len(jobs) != 1 || jobs[0].Finished() || jobs[0].Stopped {
		t.Errorf("jobs = %+v, want the one job still running", jobs)
	}
}

// A shell with no way to ask says nothing rather than guessing, and the job
// stays where it was — which is what an embedded Runner with none of the
// process hooks has always had.
func TestWithoutTheAskAResumedJobIsLeftAlone(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}, polls: []Wait{{Status: 0}}}
	_, _, r := jobSession(t, f, echoCmd+"\nbg", true, func(*Semantics, *Diagnostics) {})
	r.PollCommand = nil
	if notices := r.FinishedJobNotices(); len(notices) != 0 {
		t.Errorf("notices = %q, want none", notices)
	}
	if len(r.Jobs()) != 1 {
		t.Errorf("%d jobs, want the one still there", len(r.Jobs()))
	}
}

// A `wait` for a job nothing is waiting on waits on the *process*, which is
// the only thing that can answer: the channel a finished job closes is closed
// by whoever waited for it, and for this job that is nobody.
func TestWaitingForAResumedJobWaitsOnTheProcess(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped, {Status: 5}}}
	_, st, r := jobSession(t, f, echoCmd+"\nbg\nwait %1", true, func(*Semantics, *Diagnostics) {})
	if st != 5 {
		t.Errorf("wait reported %d, want the resumed job's 5", st)
	}
	if len(r.Jobs()) != 0 {
		t.Errorf("%d jobs, want the waited-for one forgotten", len(r.Jobs()))
	}
}

// Leaving with a job stopped: the shell that holds says so and stays, and the
// next attempt goes through.
//
// The suppression is measured rather than guessed, and it is not "warned
// once": what silences the warning is the thing immediately before it. A
// `jobs` listing does, because that is the shell showing the same thing on
// purpose; any other command does not; and a job stopping afterwards starts it
// over. A chunk is a typed line here, which is what the front end runs.
func TestAnExitIsHeldBackForAStoppedJob(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{"straight after the stop", []string{"exit"}, true},
		{"a second time, after the warning", []string{"exit", "exit"}, false},
		{"after any other command", []string{"true", "exit"}, true},
		{"after a listing", []string{"jobs", "exit"}, false},
		{"after a listing and then something else", []string{"jobs", "true", "exit"}, true},
		{"after a listing of process ids", []string{"jobs -p", "exit"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			_, _, r := jobSession(t, f, echoCmd, true, func(_ *Semantics, d *Diagnostics) {
				d.StoppedJobsAtExit = "there are stopped jobs"
			})
			for _, line := range tc.lines {
				jobRun2(t, r, line)
			}
			if held := !r.Exited(); held != tc.want {
				t.Errorf("held = %v, want %v", held, tc.want)
			}
		})
	}
}

// A shell whose dialect does not hold leaves at once, and says nothing.
func TestADialectThatDoesNotHoldTheExitLeaves(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}}
	_, _, r := jobSession(t, f, echoCmd, true, func(s *Semantics, d *Diagnostics) {
		s.StoppedJobsHoldTheExit = No
		d.StoppedJobsAtExit = "there are stopped jobs"
	})
	out, _, _ := jobRun2(t, r, "exit")
	if !r.Exited() {
		t.Error("the shell stayed, want it to leave")
	}
	if strings.Contains(out, "stopped jobs") {
		t.Errorf("said %q, want nothing", out)
	}
}

// The status a held `exit` reports is the dialect's, and the two shells that
// hold disagree about it: measured, `echo $?` after the refusal says 1 in bash
// and 0 in zsh.
func TestTheStatusOfAHeldExit(t *testing.T) {
	for _, want := range []int{0, 1} {
		f := &fakeJobs{waits: []Wait{stopped}}
		_, _, r := jobSession(t, f, echoCmd, true, func(_ *Semantics, d *Diagnostics) {
			d.StoppedJobsAtExit = "there are stopped jobs"
			d.StoppedJobsAtExitStatus = want
		})
		if _, st, _ := jobRun2(t, r, "exit"); st != want {
			t.Errorf("the held exit reported %d, want %d", st, want)
		}
		if r.Exited() {
			t.Error("the shell left, want it held back")
		}
	}
}

// Whether the command that just ran was interrupted, which is what tells the
// prompt to start on a line of its own after the `^C` the terminal echoed.
//
// Asked of the command rather than of the process: a shell that hands the
// terminal to what it runs is not in the group the ^C goes to, so it hears no
// signal at all. A stop is not an interrupt — the job is still there, and the
// notice for it is written here rather than by the prompt.
func TestWhetherTheLastCommandWasInterrupted(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    Wait
		want bool
	}{
		{"interrupted", Wait{Signal: syscall.SIGINT, Killed: true}, true},
		{"killed by something else", Wait{Signal: syscall.SIGTERM, Killed: true}, false},
		{"stopped", stopped, false},
		{"ended by itself", Wait{Status: 0}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{tc.w}}
			_, _, r := jobSession(t, f, echoCmd, false, func(*Semantics, *Diagnostics) {})
			if got := r.LastCommandWasInterrupted(); got != tc.want {
				t.Errorf("LastCommandWasInterrupted = %v, want %v", got, tc.want)
			}
		})
	}
}

// A job that is running again is not one the *stopped* warning is about:
// `bg` resumed it, nothing about it is waiting to be told to go on, and a
// shell that gave the stopped-job sentence for it would be naming the wrong
// state.
//
// It is also the option's off state, which is where both shells that have the
// name start bash: with `checkjobs` unasked for, a running job does not hold
// the exit at all. The on state is the test below.
func TestARunningJobDoesNotHoldTheExitWithTheOptionOff(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}}
	_, _, r := jobSession(t, f, echoCmd+"\nbg", true, func(_ *Semantics, d *Diagnostics) {
		d.StoppedJobsAtExit = "there are stopped jobs"
		d.RunningJobsAtExit = "there are running jobs"
	})
	if jobs := r.Jobs(); len(jobs) != 1 || jobs[0].Stopped {
		t.Fatalf("jobs = %+v, want the one job resumed", jobs)
	}
	if r.ChecksRunningJobsAtExit() {
		t.Error("a runner nobody asked starts with the running-job check on")
	}
	out, _, _ := jobRun2(t, r, "exit")
	if !r.Exited() {
		t.Error("the shell stayed for a running job, want it to leave")
	}
	if strings.Contains(out, "jobs") {
		t.Errorf("said %q, want nothing", out)
	}
}

// With the option on — bash's `shopt -s checkjobs`, zsh's `setopt checkjobs`,
// which is on there to begin with — a running job holds the exit in its own
// words, and the same told-once rules apply as for a stopped one.
//
// Measured through a pseudo-terminal against bash 5.3.15 and zsh 5.9.2 on
// 2026-09-08: `sleep 40 &` then `exit` says `There are running jobs.` /
// `you have running jobs.` and draws another prompt; a second `exit` leaves;
// a `jobs` listing before the `exit` counts as having been told; and any
// other command in between does not.
func TestAnExitIsHeldBackForARunningJob(t *testing.T) {
	for _, tc := range []struct {
		name  string
		lines []string
		want  bool
	}{
		{"straight after the job started", []string{"exit"}, true},
		{"a second time, after the warning", []string{"exit", "exit"}, false},
		{"after any other command", []string{"true", "exit"}, true},
		{"after a listing", []string{"jobs", "exit"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			_, _, r := jobSession(t, f, echoCmd+"\nbg", true, func(_ *Semantics, d *Diagnostics) {
				d.StoppedJobsAtExit = "there are stopped jobs"
				d.RunningJobsAtExit = "there are running jobs"
			})
			r.SetChecksRunningJobsAtExit(true)
			var out string
			for _, line := range tc.lines {
				out, _, _ = jobRun2(t, r, line)
			}
			if held := !r.Exited(); held != tc.want {
				t.Errorf("held = %v, want %v", held, tc.want)
			}
			if tc.want && !strings.Contains(out, "there are running jobs") {
				t.Errorf("said %q, want the running-job wording", out)
			}
		})
	}
}

// One of each: the stopped one wins the sentence.
//
// Measured, and in both shells — a session holding a suspended job and a
// `sleep 40 &` is told `There are stopped jobs.` / `you have suspended jobs.`
// and never hears about the running one, even though bash's listing under the
// sentence shows both rows.
func TestAStoppedJobWinsTheSentenceOverARunningOne(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped, stopped}}
	_, _, r := jobSession(t, f, echoCmd+"\nbg\n"+echoCmd, true, func(_ *Semantics, d *Diagnostics) {
		d.StoppedJobsAtExit = "there are stopped jobs"
		d.RunningJobsAtExit = "there are running jobs"
	})
	r.SetChecksRunningJobsAtExit(true)
	jobs := r.Jobs()
	if len(jobs) != 2 || jobs[0].Stopped || !jobs[1].Stopped {
		t.Fatalf("jobs = %+v, want one resumed and one stopped", jobs)
	}
	out, _, _ := jobRun2(t, r, "exit")
	if !r.Exited() && !strings.Contains(out, "there are stopped jobs") {
		t.Errorf("said %q, want the stopped-job wording to win", out)
	}
	if strings.Contains(out, "there are running jobs") {
		t.Errorf("said %q, want the running one not mentioned", out)
	}
}

// The stopped half can be switched off as well, and only one shell lets a
// session do it: measured, `unsetopt checkjobs` in zsh 5.9.2 leaves at the
// first `exit` with a job suspended, where `shopt -u checkjobs` in bash
// 5.3.15 still says `There are stopped jobs.` and stays. The switch is here
// and which dialect moves it is the dialect's business.
func TestTheStoppedJobCheckCanBeTurnedOff(t *testing.T) {
	f := &fakeJobs{waits: []Wait{stopped}}
	_, _, r := jobSession(t, f, echoCmd, true, func(_ *Semantics, d *Diagnostics) {
		d.StoppedJobsAtExit = "there are stopped jobs"
	})
	if !r.ChecksStoppedJobsAtExit() {
		t.Error("a runner nobody asked starts with the stopped-job check off")
	}
	r.SetChecksStoppedJobsAtExit(false)
	out, _, _ := jobRun2(t, r, "exit")
	if !r.Exited() {
		t.Error("the shell stayed with the check turned off, want it to leave")
	}
	if strings.Contains(out, "stopped jobs") {
		t.Errorf("said %q, want nothing", out)
	}
}

// The job table under the sentence: one shell writes it and the other never
// does, and the one that writes it only does so while the option is on.
//
// Both halves are asserted, because the absence is as much a fact as the
// presence — a shell that printed the table where zsh does not would be
// adding output to somebody's terminal on the way out.
func TestTheJobTableUnderTheHeldExit(t *testing.T) {
	for _, tc := range []struct {
		name    string
		lists   Answer
		checks  bool
		wantRow bool
	}{
		{"the shell that lists, with the option on", Yes, true, true},
		{"the shell that lists, with the option off", Yes, false, false},
		{"the shell that never lists", No, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &fakeJobs{waits: []Wait{stopped}}
			_, _, r := jobSession(t, f, echoCmd, true, func(s *Semantics, d *Diagnostics) {
				s.HeldExitListsTheJobs = tc.lists
				d.StoppedJobsAtExit = "there are stopped jobs"
			})
			r.SetChecksRunningJobsAtExit(tc.checks)
			out, _, _ := jobRun2(t, r, "exit")
			if !strings.Contains(out, "there are stopped jobs") {
				t.Fatalf("said %q, want the warning", out)
			}
			// The row's own word, which the sentence above does not carry.
			if got := strings.Contains(out, "Stopped"); got != tc.wantRow {
				t.Errorf("listed = %v, want %v (said %q)", got, tc.wantRow, out)
			}
		})
	}
}

// A job that has finished is not one the table under the warning lists.
//
// It is in the runner's job list until something asks for its notice, so the
// listing has to skip it rather than assume the list holds only live jobs —
// and the reason it must is the sentence itself: the warning is about jobs
// that would be *abandoned*, and one that has already ended abandons nothing.
func TestTheJobTableUnderTheHeldExitSkipsAFinishedJob(t *testing.T) {
	// A `&` job whose process ends: the goroutine waiting on it finishes the
	// job where it stands, with nothing reaping and nothing reporting, so the
	// job is in the table and done at the same time. That is the state the
	// skip is for, and it is reachable in a real session because a background
	// job can end at any moment — including between the `exit` being typed
	// and the table being built.
	f := &fakeJobs{}
	_, _, r := jobSession(t, f, echoCmd+" &", true, func(s *Semantics, d *Diagnostics) {
		s.HeldExitListsTheJobs = Yes
		d.StoppedJobsAtExit = "there are stopped jobs"
	})
	r.SetChecksRunningJobsAtExit(true)
	// Synchronized on the job rather than on a duration: the wait this
	// returns from is the one that finished it, so nothing here depends on
	// what else the machine is doing.
	for _, j := range r.Jobs() {
		j.Wait()
	}
	// And now one that stops, which is what holds the exit at all.
	f.waits = append(f.waits, stopped)
	jobRun2(t, r, echoCmd)
	jobs := r.Jobs()
	if len(jobs) != 2 || !jobs[0].Finished() || !jobs[1].Stopped {
		t.Fatalf("jobs = %+v, want one finished and one still stopped", jobs)
	}
	out, _, _ := jobRun2(t, r, "exit")
	if !strings.Contains(out, "there are stopped jobs") {
		t.Fatalf("said %q, want the warning", out)
	}
	if n := strings.Count(out, "Stopped"); n != 1 {
		t.Errorf("listed %d rows, want only the live job: %q", n, out)
	}
	if strings.Contains(out, "Done") {
		t.Errorf("listed the job that had already finished: %q", out)
	}
}
