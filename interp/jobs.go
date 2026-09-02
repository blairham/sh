// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"strings"
	"sync"
	"syscall"

	"github.com/blairham/sh/syntax"
)

// Job is a command running in the background.
type Job struct {
	// PID is the process, or 0 where the job is not one — a background
	// builtin or compound command has no process of its own, which is stated
	// rather than papered over.
	PID    int
	Status int

	// Stopped says the process is still there and waiting to be told to go
	// on — what ^Z leaves behind. A stopped job is not a finished one, and
	// the difference is the whole reason `fg` exists.
	Stopped bool

	// StopSig is what stopped it, and is 0 where nothing did.
	//
	// Kept because one dialect names it: dash lists a stopped job as
	// `Suspended: 18` rather than `Stopped`, so the number has to travel
	// with the job to reach the listing.
	StopSig int

	// Command is what was typed, for a `jobs` listing to show. Empty where
	// the shell had nothing to record — a job with no process of its own.
	Command string

	done chan struct{}
	once sync.Once
	// ready is closed once the PID is known, or once the job has finished
	// without ever having one. `$!` has to be answerable on the next line,
	// so starting the job cannot return before this is settled.
	ready     chan struct{}
	readyOnce sync.Once
}

// markReady says the job's PID is now final, one way or the other.
func (j *Job) markReady() { j.readyOnce.Do(func() { close(j.ready) }) }

// Finished reports whether the job has ended, without waiting for it.
//
// Not the same question as Stopped: a stopped job has not ended and is
// waiting to be told to go on, which is why `fg` can still name it.
func (j *Job) Finished() bool {
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// Wait blocks until the job finishes and reports its status.
func (j *Job) Wait() int {
	<-j.done
	return j.Status
}

func (j *Job) finish(status int) {
	j.markReady()
	j.once.Do(func() {
		j.Status = status
		close(j.done)
	})
}

// background starts a statement without waiting for it.
//
// An external command gets a real process group, which is what makes it a job
// rather than a goroutine. A builtin or a compound command has no process to
// group: a real shell forks a subshell there, which Go cannot do, so it runs
// on a copy of the state instead and its PID is reported as zero. That is a
// limitation worth stating rather than hiding, because the difference is
// visible the moment anything tries to signal it.
func (r *Runner) background(ctx context.Context, st *syntax.Stmt) error {
	job := &Job{
		done:  make(chan struct{}),
		ready: make(chan struct{}),
		// What was typed. The words are about to be expanded and the
		// process started, and after that nothing else remembers how the
		// command was spelled — which is what a `jobs` listing shows.
		//
		// Kept whatever the dialect will do with it. Two shells never show
		// it for a `&` job, and that is asked where the listing is built:
		// starting the job does not turn on the answer, and refusing to
		// start one over a question about how it would be *printed* would
		// be refusing to work.
		Command: st.Text,
	}

	sub := r.clone()
	sub.bg = job
	// A background job runs concurrently with everything after it, so it
	// shares the caller's streams with the foreground. That is the pipeline
	// race again in a second place: a real shell hands each side a file
	// descriptor and the kernel serializes them, and an io.Writer carries no
	// such guarantee. The shell creates the concurrency, so it guards them.
	sub.Stdout = r.lockedStdout()
	sub.Stderr = r.lockedStderr()
	go func() {
		// Errors inside a background job are reported where the job runs;
		// there is nowhere to return them to.
		if err := sub.expr(ctx, st.Expr); err != nil {
			sub.diagf("%v\n", err)
			job.markReady()
			job.finish(1)
			return
		}
		// A job that never started a process — a builtin, a compound command
		// — becomes ready when it finishes, with a PID of zero.
		job.markReady()
		job.finish(sub.status)
	}()

	// Wait for the PID to be known before returning, so `$!` on the next line
	// is not racing the goroutine that sets it.
	<-job.ready

	r.jobs = append(r.jobs, job)
	// `$!` is the most recent background job, which is how a script waits for
	// a specific one.
	r.lastJob = job
	r.announceJob(job)
	// Starting a job succeeds even when the job will not.
	r.status = 0
	return nil
}

// announceJob says a job has started, which only a shell with someone to tell
// does.
//
// The pid is settled by the time this runs: starting a job waits for it, so
// that `$!` on the next line is not racing the goroutine that sets it, and the
// announcement wants the same number.
func (r *Runner) announceJob(job *Job) {
	if !r.JobControl {
		return
	}
	if !r.ask(r.sem().AnnouncesBackgroundJob, "a background job being announced") {
		return
	}
	r.errf("%s\n", Wording(r.diag().JobStarted, "[%[1]d] %[2]d", len(r.jobs), job.PID))
}

// FinishedJobNotices is what to say about the jobs that have ended since it
// was last asked, and forgets them.
//
// For the shell around it, because *when* is not this package's to decide: a
// notice arrives before the next prompt rather than the moment the job ends,
// which is why every shell in the panel reports it after the running command
// has finished printing. The rendering is here because the wording is.
//
// Taken and forgotten, the same rule a listing follows: a job is reported
// once. Reporting it and then listing it again would be saying it twice.
func (r *Runner) FinishedJobNotices() []string {
	if !r.JobControl {
		return nil
	}
	var lines []string
	kept := r.jobs[:0]
	for i, j := range r.jobs {
		if !j.Finished() {
			kept = append(kept, j)
			continue
		}
		// The command is always shown, even in the two dialects that leave
		// it out of a `jobs` listing: both of them print it here. That is
		// what makes JobsShowBackgroundCommand a question about the listing
		// rather than about the text.
		lines = append(lines, r.jobLineAs(i, j, true, true))
	}
	for i := len(kept); i < len(r.jobs); i++ {
		r.jobs[i] = nil
	}
	r.jobs = kept
	if r.lastJob != nil && r.lastJob.Finished() {
		r.lastJob = nil
	}
	return lines
}

// biWait waits for background jobs.
//
// With no arguments it waits for all of them and reports 0, which is what
// every shell in the panel does regardless of how the jobs exited.
func biWait(r *Runner, _ context.Context, args []string) int {
	if len(args) == 0 {
		for _, j := range r.jobs {
			j.Wait()
		}
		r.jobs = nil
		return 0
	}
	last := 0
	for _, a := range args {
		pid, ok := atoi(a)
		if !ok {
			r.diagf("wait: %s: not a pid\n", a)
			return 2
		}
		for _, j := range r.jobs {
			if j.PID == pid {
				last = j.Wait()
			}
		}
	}
	return last
}

// addStoppedJob records a foreground command that stopped rather than
// finished.
//
// It is a job now: `jobs` lists it, `fg` and `bg` name it, and it holds a
// process that is still there and will stay there until something tells it to
// go on. A shell that forgot it would leave the process stopped forever with
// nothing able to name it.
func (r *Runner) addStoppedJob(pid int, argv []string, sig syscall.Signal) {
	job := &Job{
		PID:     pid,
		Stopped: true,
		StopSig: int(sig),
		Command: strings.Join(argv, " "),
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	job.markReady()
	r.jobs = append(r.jobs, job)
	r.lastJob = job
}

// Jobs is what this shell is keeping track of, oldest first.
//
// For the shell around it: a `jobs` builtin has to list them and `fg` has to
// find one by number, and the bookkeeping is this package's.
func (r *Runner) Jobs() []*Job { return r.jobs }

// Forget drops a job the shell has finished with — one that has been resumed
// into the foreground and ended, or reported as done.
func (r *Runner) Forget(j *Job) {
	for i, other := range r.jobs {
		if other == j {
			r.jobs = append(r.jobs[:i], r.jobs[i+1:]...)
			break
		}
	}
	if r.lastJob == j {
		r.lastJob = nil
	}
}
