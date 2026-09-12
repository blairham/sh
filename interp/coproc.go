// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"

	"github.com/blairham/sh/syntax"
)

// coprocClause runs `coproc [NAME] command`: the command goes to the
// background with a pipe on each of its named streams, and the shell keeps
// the near ends — one to read what the command writes, one to write what it
// reads.
//
// How a script *reaches* those ends is the dialect's, and the two shells with
// the word answer it differently. bash publishes them as the two elements of
// an array, with the process in NAME_PID, and a script writes
// `echo hi >&"${COPROC[1]}"`. zsh publishes nothing at all — it has no name
// for a coprocess and no array — and a script speaks to it with `print -p`
// and `read -p`. Measured 2026-09-05: `coproc cat; print -p hi; read -p l`
// answers `hi` there, and `${COPROC[0]}` is empty. So the ends are kept on
// the runner either way and the array is CoprocEndsInAnArray's question.
//
// Real pipes rather than in-process ones, because the command is usually an
// external process and a pipe an os/exec child inherits is a descriptor, not
// a Go value. Closing the write end through `{v}>&-` is what lets the
// command see its input finish, which is why that close is a real one.
func (r *Runner) coprocClause(ctx context.Context, c *syntax.CoprocClause) error {
	name := c.Name
	if name == "" {
		name = "COPROC"
	}
	job, err := r.startCoproc(ctx, name, func(sub *Runner) error {
		return sub.command(ctx, c.Cmd)
	})
	if err != nil || job == nil {
		return err
	}
	if r.ask(r.sem().CoprocEndsInAnArray, "a coprocess putting its ends in an array") {
		r.setArrayElem(name, 0, "0", itoa(r.coproc.read))
		r.setArrayElem(name, 1, "1", itoa(r.coproc.write))
		r.setVar(name+"_PID", itoa(job.PID))
	} else if r.unspecified {
		return nil
	}
	r.status = 0
	return nil
}

// coprocStmt runs `cmd |&`, ksh93's spelling of the same construct: a
// statement terminator rather than a word in front of a command, so what goes
// to the background is the whole and-or and there is no name to publish the
// ends under.
//
// **A second one while the first is still running is refused, and fatally.**
// Measured on ksh93u+ 2012-08-01, 2026-09-07, from a script file under
// `env -i`: two `cat |&` in a row answer `process already exists` and the
// script ends at status 1 with the line after the second one unreached. It is
// about a coprocess still *running* and not about one ever having been
// started — `true |&`, a wait, then `cat |&` is accepted, status 0.
//
// That is the opposite of what the `coproc` word does, measured the same day:
// a second `coproc cat` in bash 5.3.15 and in zsh 5.9.2 replaces the first,
// silently, at status 0 and with the script carrying on. Two constructs, two
// answers, both measured — which is why this lives on the operator's path
// rather than becoming a dialect axis over one shared path.
func (r *Runner) coprocStmt(ctx context.Context, st *syntax.Stmt) error {
	if r.coproc != nil && r.coproc.job != nil && !r.coproc.job.Finished() {
		// Named for the statement that was refused, the way a command's own
		// complaints are: nothing has run for this statement yet, so the
		// line the previous command left behind would be the wrong one.
		r.line = r.lineOf(st.Pos())
		r.fatal("%s\n", Wording(r.diag().CoprocessAlreadyRunning, "process already exists"))
		return nil
	}
	if _, err := r.startCoproc(ctx, st.Text, func(sub *Runner) error {
		return sub.expr(ctx, st.Expr)
	}); err != nil {
		return err
	}
	r.status = 0
	return nil
}

// startCoproc is the machinery both spellings share: a pipe on each of the
// command's named streams, the far ends handed to a background job and the
// near ends kept in the shell's own descriptor table.
//
// It returns the job so the caller that has a name to publish can read its
// process, and a nil job with a nil error when the pipes could not be made —
// which is already reported and already status 1.
func (r *Runner) startCoproc(ctx context.Context, name string, run func(*Runner) error) (*Job, error) {
	// What the command reads: the shell writes shellW, the command reads
	// childIn. And the reverse for what it writes.
	childIn, shellW, err := os.Pipe()
	if err != nil {
		r.diagf("%v\n", err)
		r.status = 1
		return nil, nil
	}
	shellR, childOut, err := os.Pipe()
	if err != nil {
		_ = childIn.Close()
		_ = shellW.Close()
		r.diagf("%v\n", err)
		r.status = 1
		return nil, nil
	}

	job := &Job{
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
		Command: name,
	}
	sub := r.clone()
	sub.inheritJobs(jobBoundaryBackground)
	sub.retagTrapBoundary(trapContextBackground)
	sub.bg = job
	// Its own copy of the descriptor table, as a background job takes: a
	// coprocess runs beside the shell that started it, and a descriptor the
	// script parks and then drops is dropped underneath it otherwise. See
	// ownDescriptors.
	releaseFds := sub.ownDescriptors()
	sub.Stdin = childIn
	sub.Stdout = childOut
	// Only the two named streams go through the pipes; complaints still
	// reach whoever is watching the shell.
	//
	// Both sides, as background() and procSub do, and for the reason
	// background() states: the shell carries straight on while the coprocess
	// runs, so a lock only the coprocess takes excludes nothing — and a child
	// the shell runs next is copied into the caller's writer by os/exec on a
	// goroutine with no share of it. See #735.
	sub.Stderr = r.lockedStderr()
	r.Stderr, r.Stdout = sub.Stderr, r.lockedStdout()
	// Handed over however the goroutine ended, for the reason a background
	// job's status is: the shell waits below for this job to report its
	// process, and a coprocess whose ends stayed open is a shell reading a
	// stream that will never finish. An interpreter bug here has to cost the
	// coprocess and no more.
	status := internalErrorStatus
	r.spawn(func() {
		if err := run(sub); err != nil {
			sub.diagf("%v\n", err)
		}
		status = sub.status
	}, func() {
		// The command is done with its ends, and closing them here is what
		// turns its exit into end-of-file for whoever reads NAME[0].
		_ = childIn.Close()
		_ = childOut.Close()
		releaseFds()
		job.finish(status)
	})
	<-job.ready

	r.jobs = append(r.jobs, job)
	r.setLastJob(job)

	// The near ends go into the descriptor table the way `exec {fd}>f`
	// would put them there: numbered from ten up, for keeps.
	// Marked as the shell's own, so they stay out of an external child's
	// descriptor table — see shellOwnedFd for what a child holding the write
	// end open would cost the coprocess.
	rfd := r.nextFreeFd()
	r.setFd(rfd, shellOwnedFd{shellR})
	wfd := r.nextFreeFd()
	r.setFd(wfd, shellOwnedFd{shellW})
	// Kept whichever dialect this is: `print -p` and `read -p` need them in
	// the shell that has no array to find them in, and the shell that has one
	// loses nothing by the record. A second `coproc` replaces the first,
	// which is what the shell with the letters does — measured, the second
	// one is the one `print -p` reaches.
	r.coproc = &coprocEnds{read: rfd, write: wfd, job: job}
	return job, nil
}

// coprocEnds is the pair of descriptors a running coprocess is reached by,
// with the job that is running behind them.
//
// The job is kept because one spelling asks whether the coprocess is still
// alive rather than whether one was ever started: ksh93's `|&` refuses a
// second while the first runs and accepts one after it has ended.
type coprocEnds struct {
	read, write int
	job         *Job
}

// CoprocRead and CoprocWrite are the descriptors of the running coprocess, for
// a dialect builtin that speaks to one by a letter rather than through an
// array — `print -p` and `read -p`. The second result is false when no
// coprocess has been started, which is the refusal both letters measure.
func (r *Runner) CoprocRead() (int, bool) {
	if r.coproc == nil {
		return 0, false
	}
	return r.coproc.read, true
}

func (r *Runner) CoprocWrite() (int, bool) {
	if r.coproc == nil {
		return 0, false
	}
	return r.coproc.write, true
}
