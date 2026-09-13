// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"errors"
	"io"
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
		err := sub.command(ctx, c.Cmd)
		// A coprocess is a subshell too, and this is where it ends.
		sub.endSubshell(ctx)
		return err
	})
	if err != nil || job == nil {
		return err
	}
	if r.ask(r.sem().CoprocEndsInAnArray, "a coprocess putting its ends in an array") {
		r.setArrayElem(name, 0, "0", itoa(r.coproc.read))
		r.setArrayElem(name, 1, "1", itoa(r.coproc.write))
		r.setVar(name+"_PID", itoa(job.PID))
		// Kept so that the reaping can take back exactly what was published.
		// See Semantics.ReapedCoprocessEnds and forgetCoprocNames.
		r.coproc.name = name
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
		done:     make(chan struct{}),
		ready:    make(chan struct{}),
		started:  make(chan struct{}),
		stopNote: make(chan struct{}),
		// The body itself, released when its pid settles either way — the
		// same count `&` keeps, for the same reason. See Job.expectPart.
		parts:   1,
		Command: name,
	}
	sub := r.clone()
	sub.inheritJobs(jobBoundaryBackground)
	sub.retagTrapBoundary(trapContextBackground)
	sub.bg = job
	sub.inJob = job
	sub.part = nil
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
	<-job.started

	r.addJob(job)
	r.setLastJob(job)
	r.becomeCurrentJob(job)

	// The near ends go into the descriptor table the way `exec {fd}>f`
	// would put them there: numbered from ten up, for keeps.
	// Marked as the shell's own, so they stay out of an external child's
	// descriptor table — see shellOwnedFd for what a child holding the write
	// end open would cost the coprocess.
	rfd := r.nextFreeFd(-1)
	r.setFd(rfd, shellOwnedFd{shellR})
	wfd := r.nextFreeFd(-1)
	r.setFd(wfd, shellOwnedFd{shellW})
	// Kept whichever dialect this is: `print -p` and `read -p` need them in
	// the shell that has no array to find them in, and the shell that has one
	// loses nothing by the record. A second `coproc` replaces the first,
	// which is what the shell with the letters does — measured, the second
	// one is the one `print -p` reaches.
	r.coproc = &coprocEnds{read: rfd, write: wfd, job: job, owner: r}
	return job, nil
}

// retireCoproc lets go of a coprocess that has ended, in the way the dialect
// says a reaped coprocess's ends are let go of.
//
// **It is called where the shell waits for a child, and nowhere else**, which
// is measured rather than convenient: the notice is not delivered by `wait`
// and it is not delivered by the next command either. Measured 2026-09-12 on
// bash 5.3.15, `coproc CP { echo hi; }` followed by each of these and then by
// `${#CP[@]}`:
//
//	:            2      a hundred builtins, and it is still 2
//	jobs         2      asking after the jobs is not waiting for one
//	( : )        0      a subshell is a fork and a wait
//	: | :        0      so is a pipeline element
//	/usr/bin/…   0      so is an external command
//	wait         0
//
// So the trigger is the shell reaping *anything*, and that is a deterministic
// stand-in for a rule that is not: bash's notice rides on SIGCHLD arriving
// whenever the child gets round to exiting and landing at the next command
// boundary after that, so a construct wins the race by taking long enough
// *and* running commands. Five iterations of `for ((i=0;i<n;i++)); do :;
// done` answer 2 five times running and five hundred answer 0 five times
// running, with no fork in either; `read -t 1` spends a whole second in one
// command and answers 2. The four places here match every shape bash answers
// the same way twice, and the long-loop shape it also answers the same way
// twice is the one they miss — recorded in docs/spec/semantics.md and filed
// as #2468, because following bash there costs the short-loop shape.
//
// Placing it at the top of every statement instead would have retired a
// coprocess before the very next line could read what it wrote — `coproc CP {
// echo hi; }; read -r a <&${CP[0]}` answers `hi` in bash and would have
// answered an ambiguous redirect here.
//
// The main thread is the only thread that runs it. The reaping itself happens
// on the goroutine running the coprocess body, and all this reads of that is
// Job.Finished, which is a closed channel; every table it then edits — the
// descriptors, the variables — belongs to this runner and is touched here
// alone. Doing the unset on the reaping goroutine would be a data race on
// Runner.Vars against whatever the script is doing at the time.
func (r *Runner) retireCoproc() {
	c := r.coproc
	if c == nil || c.owner != r || c.retired || c.job == nil || !c.job.Finished() {
		return
	}
	c.retired = true
	switch r.sem().ReapedCoprocessEnds {
	case CoprocWriteEndGoesWithTheCoprocess:
		// The read end stays: what the coprocess wrote before it ended is
		// still in the pipe, and a script still reads it. It goes when that
		// read finds nothing left — see coprocReadEnded, which is not this
		// axis's to decide.
		r.forgetCoprocFd(c.write)
		c.write = -1
	case CoprocEndsGoWithTheCoprocess:
		r.forgetCoprocFd(c.read)
		r.forgetCoprocFd(c.write)
		r.forgetCoprocNames()
		// Nothing is left to speak to, so the letters that reach a coprocess
		// by name find none running — which is the same refusal they give
		// before any coprocess was started.
		r.coproc = nil
	}
}

// coprocReadEnded is the end-of-file a read of the coprocess's near end found,
// and it is the whole coprocess that goes there rather than only that end.
//
// **Shared ground rather than an axis**, measured 2026-09-12 on both shells
// that have the letters, with a coprocess writing one line and exiting. The
// first `read -p` answers the line, the second is a silent end-of-file at 1,
// and the *third* is `-p: no coprocess` in zsh 5.9.2 and `read: no query
// process` in ksh93 — and so is a `print -p` after it, which is what says the
// write end went with the read one rather than the read end alone. Neither
// shell needs the coprocess reaped first: the same three answers come back
// with no `wait` anywhere and the reaping unobserved.
//
// bash never reaches this, and that is measured too rather than assumed: it
// spells `-p` as a prompt and reads its coprocess through the array, and two
// reads that both find end-of-file leave `${#CP[@]}` at 2. So the array is
// taken back by the reaping alone — see Semantics.ReapedCoprocessEnds — and
// nothing here has to ask which dialect it is in.
//
// Reported by the read rather than noticed here, because end-of-file is not a
// state a descriptor is in: it is what a read came back with, and nothing else
// in this shell is reading that pipe.
func (r *Runner) coprocReadEnded() {
	c := r.coproc
	if c == nil || c.owner != r || c.readEnded {
		return
	}
	c.readEnded = true
	r.forgetCoprocFd(c.read)
	r.forgetCoprocFd(c.write)
	r.coproc = nil
}

// forgetCoprocFd takes one near end out of the descriptor table.
//
// The entry goes and the file does not get closed here, which is deliberate:
// a script may have duplicated the end onto a number of its own with `exec
// 3>&${CP[1]}`, and that duplicate is the same open file. Measured 2026-09-12
// on bash 5.3.15 — after the coprocess is reaped, a write through such a 3 is
// still a write into a pipe with no reader and still ends the shell on
// SIGPIPE, which is exactly what closing the file underneath it would turn
// into a complaint instead. An end nothing else names is unreferenced once
// the entry goes, and the file's own cleanup is what reclaims it.
//
// A number saved out of the array as a *number* is not a duplicate and is not
// protected by any of that: `r=${CP[0]}` and a later `<&$r` finds nothing in
// the table and is `Bad file descriptor`, which is what bash answers.
func (r *Runner) forgetCoprocFd(fd int) {
	if fd < 0 {
		return
	}
	delete(r.fds, fd)
}

// forgetCoprocNames unsets whatever published the ends.
//
// Only the dialect with an array has anything to unset, and it is asked as the
// same question that put the names there: where the ends are published in an
// array, the array is how a script reaches them, so an array outliving the
// ends would name two descriptors that are gone.
//
// Unset rather than emptied, measured: `declare -p CP` after the reaping
// answers `CP: not found` rather than an empty array, and `${CP+set}` is
// empty. The redirection then has an empty word for a target, which is the
// `ambiguous redirect` a script sees.
func (r *Runner) forgetCoprocNames() {
	if r.coproc == nil || r.coproc.name == "" {
		return
	}
	if r.sem().CoprocEndsInAnArray != Yes {
		return
	}
	r.unsetName(r.coproc.name)
	r.unsetName(r.coproc.name + "_PID")
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

	// name is what the array publishing the ends is called, and is empty for
	// the two spellings that publish nothing — zsh's `coproc`, which has no
	// name for a coprocess, and ksh93's `|&`, which has no word in front of
	// it to carry one. It is recorded when the array is written rather than
	// when the coprocess starts, so the name here is exactly the name that
	// was published and never one this shell declined to publish under.
	name string

	// owner is the runner that started it, and is what makes retireCoproc
	// safe to call from every statement. A subshell is a shallow copy of its
	// parent, so it holds this same pointer and would otherwise retire the
	// *parent's* coprocess — unsetting an array and dropping descriptors in a
	// runner that is about to be discarded, while the shell that owns them
	// still holds both. A coprocess started inside a subshell is owned by
	// that subshell and retires there, which is the same rule read the other
	// way.
	owner *Runner

	// retired records that the ends have already been let go of, so a
	// coprocess is retired once rather than at the top of every statement
	// after it ended. It also keeps the write end's disposal from being
	// undone by the read end's, which arrives later in one dialect.
	retired bool

	// readEnded records that a read of the near end has reached end-of-file,
	// which is when both dialects with the coprocess letters let go of the
	// whole coprocess. See coprocReadEnded, and note that this is not the
	// reaping: it happens with the coprocess still unreaped and it happens
	// whatever ReapedCoprocessEnds says.
	readEnded bool
}

// CoprocRead and CoprocWrite are the descriptors of the running coprocess, for
// a dialect builtin that speaks to one by a letter rather than through an
// array — `print -p` and `read -p`. The second result is false when no
// coprocess has been started, which is the refusal both letters measure.
//
// An end the reaping already took back answers false as well, and it is the
// *same* refusal rather than a second one: measured 2026-09-12, ksh93 answers
// `print: no query process` word for word whether no coprocess was ever
// started or the one that was has ended. The two ends are asked separately
// because that dialect lets go of them separately — see
// Semantics.ReapedCoprocessEnds.
func (r *Runner) CoprocRead() (int, bool) {
	if r.coproc == nil || r.coproc.read < 0 {
		return 0, false
	}
	return r.coproc.read, true
}

func (r *Runner) CoprocWrite() (int, bool) {
	if r.coproc == nil || r.coproc.write < 0 {
		return 0, false
	}
	return r.coproc.write, true
}

// coprocReader is the near end a `read -p` reads, watching for the end of it.
//
// It exists because the dialect that keeps a reaped coprocess's read end lets
// go of it on end-of-file rather than on the reaping, and nothing else in this
// shell is reading that pipe — so the read that found the end is the only
// thing that can say so. See Runner.coprocReadEnded.
type coprocReader struct {
	io.Reader
	r *Runner
}

func (c *coprocReader) Read(p []byte) (int, error) {
	n, err := c.Reader.Read(p)
	if errors.Is(err, io.EOF) {
		c.r.coprocReadEnded()
	}
	return n, err
}
