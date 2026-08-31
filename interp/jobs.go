// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"sync"

	"github.com/blairham/sh/syntax"
)

// Job is a command running in the background.
type Job struct {
	// PID is the process, or 0 where the job is not one — a background
	// builtin or compound command has no process of its own, which is stated
	// rather than papered over.
	PID    int
	Status int

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
	job := &Job{done: make(chan struct{}), ready: make(chan struct{})}

	sub := r.clone()
	sub.bg = job
	// A background job runs concurrently with everything after it, so it
	// shares the caller's streams with the foreground. That is the pipeline
	// race again in a second place: a real shell hands each side a file
	// descriptor and the kernel serializes them, and an io.Writer carries no
	// such guarantee. The shell creates the concurrency, so it guards them.
	sub.Stdout = &lockedWriter{w: r.stdout()}
	sub.Stderr = &lockedWriter{w: r.stderr()}
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
	// Starting a job succeeds even when the job will not.
	r.status = 0
	return nil
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
