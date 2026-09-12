// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Real commands, because only an *external* one goes through the path that
// waits — a builtin never starts a process and so can never stop.
//
// Silent ones, and that is not incidental. A command the shell has been told
// *stopped* is still running as far as os/exec is concerned, so its stdio
// copier is still live and still writing; a test that then read the buffer it
// writes to would be racing it, which the race detector duly found. Nothing
// here prints, so there is nothing to race over.
var (
	echoCmd = lookCmd("true")
	lsCmd   = lookCmd("false")
)

func lookCmd(name string) string {
	for _, dir := range []string{"/usr/bin/", "/bin/"} {
		if _, err := os.Stat(dir + name); err == nil {
			return dir + name
		}
	}
	return name
}

// A shell that can be told a command *stopped* rather than finished.
//
// The hooks stand in for the process here: nothing is started, nothing is
// signaled, and the tests can say exactly what the kernel would have reported
// — including the case a real terminal makes hard to arrange on demand.
type fakeJobs struct {
	waits   []Wait
	signals []struct {
		pgid int
		sig  syscall.Signal
	}
	foreground []int
	// polls are what the shell is told when it asks after a job nothing is
	// waiting on — what `bg` let go of. Empty means nothing has changed,
	// which is the answer for a job that is still running.
	polls []Wait
	// saidStopped is every process this fake claimed had stopped. See
	// waitFor and reapSaidStopped: the claim is a lie about a process that
	// has in fact exited, and something has to reap it.
	saidStopped []int
}

// waitFor is this fake standing in for the front end's `waitpid`.
//
// The signature is Runner.WaitForCommand's, and the difference from the real
// thing is the whole of #1006. A front end's wait *reaps*: it is a waitpid,
// and the child is gone from the process table when it returns. This one
// returns a canned answer and calls nothing, so the child is only reaped by
// whatever runs afterwards.
//
// For every answer but a stop, that is the shell: runWatched calls cmd.Wait
// once the wait it was given says the command ended. A stop is the one answer
// where it deliberately does not, and it is right not to — a stopped job is
// still alive and is waited for again when it resumes. But the child here did
// not stop. It ran `true` and exited, so declining to reap it leaves a zombie,
// and a clean `go test ./interp/` ended with dozens of them.
//
// So the fake reaps what the fake lied about. Recorded here and waited for in
// reapSaidStopped rather than reaped on the spot, because a real waitpid
// returns when the child changes state and this call is on the shell's own
// thread of control — blocking it would be modeling something no front end
// does.
func (f *fakeJobs) waitFor(pid int) (Wait, error) {
	w := f.next()
	if w.Stopped {
		f.saidStopped = append(f.saidStopped, pid)
	}
	return w, nil
}

// reapSaidStopped waits for the children this fake told the shell were
// stopped, so the run does not end with a process table full of them.
//
// Polled rather than blocking. Every command these tests start is `true`, so
// the wait is over before the first look; a blocking wait4 on a child that
// somehow had not exited would turn a leak into a ten-minute test timeout,
// which is a worse failure than the one being fixed.
func (f *fakeJobs) reapSaidStopped(t *testing.T) {
	t.Helper()
	for _, pid := range f.saidStopped {
		deadline := time.Now().Add(5 * time.Second)
		for {
			var ws syscall.WaitStatus
			got, err := syscall.Wait4(pid, &ws, syscall.WNOHANG, nil)
			if got == pid || errors.Is(err, syscall.ECHILD) {
				// Reaped, or somebody else already had it.
				break
			}
			if err != nil && !errors.Is(err, syscall.EINTR) {
				t.Errorf("waiting for %d, which this fake said had stopped: %v", pid, err)
				break
			}
			if !time.Now().Before(deadline) {
				t.Errorf("process %d, which this fake said had stopped, is still running", pid)
				break
			}
			time.Sleep(time.Millisecond)
		}
	}
	f.saidStopped = nil
}

func (f *fakeJobs) next() Wait {
	if len(f.waits) == 0 {
		return Wait{}
	}
	w := f.waits[0]
	f.waits = f.waits[1:]
	return w
}

// poll is the non-blocking half: a Wait and whether anything happened.
func (f *fakeJobs) poll() (Wait, bool) {
	if len(f.polls) == 0 {
		return Wait{}, false
	}
	w := f.polls[0]
	f.polls = f.polls[1:]
	return w, true
}

// jobRun runs src on a shell whose job control is the fake's, and hands back
// the runner so more input can be run on the same jobs.
//
// tweak, where given, moves the semantics before the run. It is variadic
// rather than a second helper because the alternative is a copy of these
// thirty lines with one line changed, and the copy is what drifts.
func jobRun(t *testing.T, f *fakeJobs, src string, tweak ...func(*Semantics)) (string, int, *Runner) {
	t.Helper()
	file, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	// A real file rather than a buffer, and that is the point rather than
	// convenience: os/exec hands a *os.File to the child directly and copies
	// anything else on a goroutine of its own. A command the shell has been
	// told *stopped* is still running as far as os/exec knows, so that
	// goroutine is still live — and a test reading the buffer it writes to is
	// racing it, which the race detector duly found. A shell's streams are
	// files, so this is also what the real thing does.
	out := sink(t)
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = No
	// Which end a listing starts from is a conflict, so the core refuses it
	// where there is more than one job. These tests are about the markers
	// and the states rather than the order, so they name an answer.
	sem.JobsListNewestFirst = No
	sem.JobsListFinishedJobs = Yes
	sem.JobsShowBackgroundCommand = Yes
	for _, f := range tweak {
		f(&sem)
	}
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Diagnostics: &dg, Name: "testsh"})
	if f != nil {
		t.Cleanup(func() { f.reapSaidStopped(t) })
		r.WaitForCommand = f.waitFor
		r.SignalGroup = func(pgid int, sig syscall.Signal) error {
			f.signals = append(f.signals, struct {
				pgid int
				sig  syscall.Signal
			}{pgid, sig})
			return nil
		}
		r.Foreground = func(pgid int) error {
			f.foreground = append(f.foreground, pgid)
			return nil
		}
		r.PollCommand = func(int) (Wait, bool, error) {
			w, changed := f.poll()
			return w, changed, nil
		}
	}
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return read(t, out), r.ExitStatus(), r
}

// sink is a file the shell writes to, and read is what it wrote.
func sink(t *testing.T) *os.File {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "out")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func read(t *testing.T, f *os.File) string {
	t.Helper()
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A command that stopped becomes a job. The status is the signal's, and the
// process is still there — which is the whole difference from one that ended.
func TestAStoppedCommandBecomesAJob(t *testing.T) {
	f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
	// The command alone first: `jobs` after it would report its own 0 and
	// hide the status the stopped command left.
	_, st, r := jobRun(t, f, echoCmd)
	if st != 128+int(syscall.SIGTSTP) {
		t.Errorf("status %d, want 128+SIGTSTP", st)
	}
	out, _, _ := jobRun2(t, r, "jobs")
	jobs := r.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want 1", len(jobs))
	}
	if !jobs[0].Stopped {
		t.Error("the job is not marked stopped")
	}
	if !strings.Contains(out, "Stopped") || !strings.Contains(out, echoCmd) {
		t.Errorf("jobs said %q, want the state and the command", out)
	}
	// The terminal went to the command and came back.
	if len(f.foreground) != 2 || f.foreground[1] != 0 {
		t.Errorf("terminal handoff was %v, want it given and taken back", f.foreground)
	}
}

// A command that ended is not a job, however it ended.
func TestAFinishedCommandIsNotAJob(t *testing.T) {
	for _, w := range []Wait{
		{Status: 0},
		{Status: 3},
		{Signal: syscall.SIGKILL, Killed: true},
	} {
		f := &fakeJobs{waits: []Wait{w}}
		_, _, r := jobRun(t, f, echoCmd)
		if len(r.Jobs()) != 0 {
			t.Errorf("%+v left %d jobs, want none", w, len(r.Jobs()))
		}
	}
}

// The status a stopped or killed command reports goes through the same axis a
// killed one already did, rather than a second copy of the arithmetic.
func TestTheStatusOfAStoppedCommand(t *testing.T) {
	f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
	_, st, _ := jobRun(t, f, echoCmd)
	if st != 128+int(syscall.SIGTSTP) {
		t.Errorf("status %d, want 128+%d", st, syscall.SIGTSTP)
	}
	// ksh93 counts from 256, and a stopped command is not an exception.
	file, err := syntax.Parse(echoCmd, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := permissive()
	sem.SignalDeathStatusIsTwoFiftySix = Yes
	dg := Diagnostics{}
	out := sink(t)
	r := newTestRunner(t, &Runner{Stdout: out, Stderr: out, Semantics: &sem, Diagnostics: &dg})
	// Through a fakeJobs rather than a closure of its own, so that this half
	// reaps what it claims stopped like the other half does. Written inline it
	// was the last zombie left in the package after #1006.
	countsFrom256 := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
	t.Cleanup(func() { countsFrom256.reapSaidStopped(t) })
	r.WaitForCommand = countsFrom256.waitFor
	if _, err := r.Run(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	if got := r.ExitStatus(); got != 256+int(syscall.SIGTSTP) {
		t.Errorf("status %d, want 256+%d", got, syscall.SIGTSTP)
	}
}

// `fg` resumes a job and waits for it; `bg` resumes it and does not.
func TestFgAndBg(t *testing.T) {
	f := &fakeJobs{waits: []Wait{
		{Signal: syscall.SIGTSTP, Stopped: true}, // the command stops
		{Status: 7},                              // and then fg waits for it
	}}
	out, st, r := jobRun(t, f, echoCmd+"\nfg")
	if st != 7 {
		t.Errorf("fg reported %d, want the resumed command's 7", st)
	}
	if len(r.Jobs()) != 0 {
		t.Error("the job was not forgotten after it finished")
	}
	if !strings.Contains(out, echoCmd) {
		t.Errorf("fg said %q, want the command named", out)
	}
	if len(f.signals) != 1 || f.signals[0].sig != syscall.SIGCONT {
		t.Errorf("signals were %+v, want one SIGCONT", f.signals)
	}

	// bg resumes and returns without waiting, so the job stays.
	f = &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
	out, st, r = jobRun(t, f, echoCmd+"\nbg")
	if st != 0 {
		t.Errorf("bg reported %d, want 0", st)
	}
	if len(r.Jobs()) != 1 || r.Jobs()[0].Stopped {
		t.Error("bg should leave a job that is no longer stopped")
	}
	if !strings.Contains(out, "&") {
		t.Errorf("bg said %q, want the & that says it is not waiting", out)
	}
}

// A job spec names one: by number, or by the markers a listing shows.
func TestJobSpecs(t *testing.T) {
	f := &fakeJobs{waits: []Wait{
		{Signal: syscall.SIGTSTP, Stopped: true},
		{Signal: syscall.SIGTSTP, Stopped: true},
	}}
	_, _, r := jobRun(t, f, echoCmd+"\n"+lsCmd)
	if len(r.Jobs()) != 2 {
		t.Fatalf("%d jobs, want 2", len(r.Jobs()))
	}
	for _, c := range []struct{ spec, want string }{
		{"%1", echoCmd},
		{"%2", lsCmd},
		{"%%", lsCmd},
		{"%+", lsCmd},
		{"%-", echoCmd},
		{"1", echoCmd},
	} {
		out, st, _ := jobRun2(t, r, "jobs "+c.spec)
		if st != 0 || !strings.Contains(out, c.want) {
			t.Errorf("%s gave %q status %d, want %q", c.spec, out, st, c.want)
		}
	}
	// One that names nothing is a failure with a complaint, not a silent 0.
	out, st, _ := jobRun2(t, r, "jobs %9")
	if st == 0 || !strings.Contains(out, "no such job") {
		t.Errorf("%%9 gave %q status %d, want a complaint", out, st)
	}
}

// jobRun2 runs more input on a runner that already has jobs.
func jobRun2(t *testing.T, r *Runner, src string) (string, int, *Runner) {
	t.Helper()
	file, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	out := sink(t)
	saved, savedErr := r.Stdout, r.Stderr
	r.Stdout, r.Stderr = out, out
	defer func() { r.Stdout, r.Stderr = saved, savedErr }()
	if err := r.RunPart(context.Background(), file); err != nil {
		t.Fatal(err)
	}
	return read(t, out), r.ExitStatus(), r
}

// Without the hooks nothing changes: a shell embedded in another program waits
// the ordinary way and has no jobs, which is what it had before any of this.
func TestWithoutTheHooks(t *testing.T) {
	_, _, r := jobRun(t, nil, echoCmd)
	if len(r.Jobs()) != 0 {
		t.Errorf("%d jobs with no hooks, want none", len(r.Jobs()))
	}
}

// The marker says which job `%%` and `%-` mean, so a listing can be read
// without knowing the order they were started in.
func TestTheListingMarkers(t *testing.T) {
	f := &fakeJobs{waits: []Wait{
		{Signal: syscall.SIGTSTP, Stopped: true},
		{Signal: syscall.SIGTSTP, Stopped: true},
		{Signal: syscall.SIGTSTP, Stopped: true},
	}}
	_, _, r := jobRun(t, f, echoCmd+"\n"+lsCmd+"\n"+echoCmd)
	out, _, _ := jobRun2(t, r, "jobs")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Fatalf("listed %d jobs, want 3:\n%s", len(lines), out)
	}
	// The newest is `+` — the one `fg` would take — and the one before it is
	// `-`. Everything older carries neither.
	if !strings.HasPrefix(lines[2], "[3]+") {
		t.Errorf("newest is %q, want [3]+", lines[2])
	}
	if !strings.HasPrefix(lines[1], "[2]-") {
		t.Errorf("the one before is %q, want [2]-", lines[1])
	}
	if !strings.HasPrefix(lines[0], "[1] ") {
		t.Errorf("the oldest is %q, want [1] with no marker", lines[0])
	}
}

// `kill` takes a job spec as well as a pid, and a job is a process *group* —
// which is what makes `kill %1` reach a pipeline rather than only its first
// command.
//
// Two processes in the group is the only arrangement that can tell the two
// apart: signaling the leader alone leaves the other running, and the test
// would pass on a bug.
func TestKillTakesAJobSpecAndReachesTheGroup(t *testing.T) {
	sh := lookCmd("sh")
	f := &fakeJobs{waits: []Wait{{Signal: syscall.SIGTSTP, Stopped: true}}}
	// A shell that starts a second process and waits: both are in the group
	// the shell put the job in.
	_, _, r := jobRun(t, f, sh+" -c '"+lookCmd("sleep")+" 30 & "+lookCmd("sleep")+" 30'")
	jobs := r.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs, want 1", len(jobs))
	}
	pgid := jobs[0].PID

	// A spec that names nothing fails; the one that names this job does not.
	if _, st, _ := jobRun2(t, r, "kill -TERM %9"); st == 0 {
		t.Error("kill %9 reported success, want a failure")
	}
	out, st, _ := jobRun2(t, r, "kill -TERM %1")
	if st != 0 {
		t.Errorf("kill %%1 reported %d saying %q, want it to find the job", st, out)
	}

	// It went to the *group* and not to one process, which is the whole point
	// of a job spec: the hook that reaches a group is the one that was
	// called, with the job's own pid as the group.
	if len(f.signals) != 1 {
		t.Fatalf("sent %d group signals, want one", len(f.signals))
	}
	if f.signals[0].pgid != pgid {
		t.Errorf("signaled group %d, want the job's %d", f.signals[0].pgid, pgid)
	}
	if f.signals[0].sig != syscall.SIGTERM {
		t.Errorf("sent %v, want SIGTERM", f.signals[0].sig)
	}
	// And tidy up: the fake did not really signal anything.
	_ = syscall.Kill(-pgid, syscall.SIGKILL)
}
