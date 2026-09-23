// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"os"
	"sync/atomic"
)

// Running a command *beside* the shell, on descriptors the caller chose.
//
// This is the public half of the machinery `coproc` was built on, and it is
// public because a dialect could not reach it. A dialect has three extension
// points — the vectors, a registered builtin, a sourced prelude — and a
// builtin is handed a [Runner] with nothing on it that starts a command
// concurrently: [Runner.Run] is synchronous, [Runner.Jobs] only lists, and
// startCoproc is this package's own. So a facility whose whole shape is "run
// this with those descriptors and keep a handle on it" had no way in, and the
// first one to want it — zsh's `zpty`, which runs a command on a
// pseudo-terminal — could not be written at all. See docs/spec/pty.md.
//
// **One mechanism, not a second one beside the first.** startCoproc already
// half-implemented this with pipes, so what is here is that function
// generalized: both spellings of `coproc` and every caller of this seam go
// through startBeside, and a fix to the boundary lands once. The alternative —
// a parallel path for the terminal case — is the shape endSubshell's own
// comment warns about, where the next fix goes missing from one of two copies.
//
// # What the caller supplies and what this owns
//
// The caller supplies the command's **own** ends: the far side of a pipe, the
// terminal side of a pseudo-terminal pair. Whether they are closed when the
// command finishes is the caller's to say — see ConcurrentCommand.CloseEnds,
// where the two consumers want opposite answers for measured reasons. Where
// they are, the order is the job first and the ends after it, because closing
// them is what turns the command's exit into end-of-file for whoever is
// reading the near end, and a reader that saw the end of input before the job
// said it had ended would be racing the bookkeeping rather than reading it.
// That ordering is the one #2661 was about and it is kept here for the same
// reason.
//
// What the shell keeps — the near end of the pipe, the control side of the
// terminal — is never handed here and is never touched here. It is the
// caller's, and where a script has to reach it by number the caller puts it in
// the descriptor table with [Runner.OpenNearEnd], which is
// [Runner.OpenDescriptor] with the two exclusions the shell's own plumbing
// needs.
//
// # The name, and what a subshell does with it
//
// A started command is kept on the runner under its name, and the table is
// **owned by a subshell while the commands in it are shared** — the same
// arrangement `jobs` has, and it is arrived at from a measurement rather than
// from symmetry. Measured on zsh 5.9.2, 2026-09-20, with a live `zpty` command
// named P:
//
//	( zpty )        lists it, status 0
//	( zpty -d P )   status 0, and the process is gone
//	zpty -t P       status 1 — the parent agrees the command has ended
//	zpty -d P       status 0 — and the parent still has the **name**
//
// A cloned map with shared values is exactly those four rows: the subshell
// sees the entry, its [Concurrent.Stop] reaches the one running command, and
// the name it forgets is its own copy of the name. Sharing the map would have
// made the last row `no such pty command`; copying the values would have left
// the process running.

// ConcurrentCommand is a command to run beside the shell.
//
// The zero value runs nothing: Run is what the command is, and a nil one is a
// caller that has not said.
type ConcurrentCommand struct {
	// Name is what the command is kept under, and what a `jobs` listing
	// would call it. It has to be unique among the commands this runner is
	// already keeping — see [Runner.StartConcurrent], which refuses a name
	// that is taken rather than losing the command already under it.
	Name string

	// In, Out and Err are the command's own three descriptors. A nil one is
	// the shell's own stream, serialized against the shell's other writers
	// the way a background job's is — which is what `coproc` wants for its
	// diagnostics, since only its two named streams go through the pipes.
	In, Out, Err *os.File

	// CloseEnds asks for each distinct one of those to be closed when the
	// command finishes. Off by default: what the caller handed over stays
	// the caller's, which is the only safe answer for a seam that cannot
	// know what else holds the other side.
	//
	// **The two consumers want opposite answers and the reason is measured.**
	// A pipe's far end has to close, or whoever reads the near end never sees
	// end-of-file and a `coproc` that has exited leaves a reader waiting. A
	// *pseudo-terminal's* far end must not: measured on macOS 2026-09-21,
	// writing to the terminal side and then closing it **discards whatever
	// the control side had not yet read**, so a command that printed and
	// exited would read back as a command that printed nothing. So `zpty`
	// keeps its terminal end until the entry is deleted and stops its reads
	// on the command having finished instead.
	CloseEnds bool

	// Run is the command. It is called on a subshell of the runner that
	// started it, on a goroutine of the shell's own, and the subshell is
	// ended for it — so the body is the running and nothing else.
	//
	// A function rather than shell text, because what a caller wants to run
	// differs: `coproc` has a parsed [syntax.Command] in hand, and a builtin
	// that read its operands as text has a [syntax.File] it parsed itself.
	// Either is [Runner.RunPart] away on the runner this is handed.
	Run func(ctx context.Context, sub *Runner) error
}

// Concurrent is a command running beside the shell — what
// [Runner.StartConcurrent] hands back and what [Runner.ConcurrentNamed] finds
// again.
type Concurrent struct {
	name string
	job  *Job
	// stop ends the run: it cancels the context the body and every external
	// command under it were started with, which is the nearest thing this
	// shell has to the signal a real one would send. A goroutine is not a
	// process and cannot be signaled; what it can be is asked to stop at the
	// next command, and an os/exec child of it is killed outright because
	// that is what exec.CommandContext does with a canceled context.
	stop context.CancelFunc
	// stopped records that Stop has been asked for, which is what makes
	// Running answerable at once. See Stop.
	stopped atomic.Bool
}

// Name is what the command was started under.
func (c *Concurrent) Name() string { return c.name }

// Ident is the number a script names this command by — a real process id
// where the command became one, and a number this shell invented where it did
// not. See jobident.go: a listing has to print something either way, and a
// command that is a goroutine here would be a process in a real shell.
func (c *Concurrent) Ident() int { return c.job.Ident() }

// Running reports whether the command is still going.
//
// It is the question `zpty -t` asks, and it is deliberately not "does the name
// exist": those are two answers with two statuses in the shell that asks, and
// a caller that collapsed them would lose the difference between a command
// that has ended and a name nobody started.
//
// **A command that has been asked to stop is not running**, from the moment it
// is asked. That is the only deterministic answer available here and it is the
// right one for a script: a command stopped here is stopped by canceling the
// context its body runs under, so there is no process to ask after and the
// goroutine ends whenever it next looks. A caller that reported "still
// running" to the line after the one that killed it would be answering about
// this implementation's scheduling rather than about the shell.
func (c *Concurrent) Running() bool { return !c.stopped.Load() && !c.job.Finished() }

// Stop asks the command to end, and is safe twice and safe on one that has
// already finished.
//
// It does not wait, and [Concurrent.Running] is false from here on — see
// there. A command that is between commands stops at the next one; an external
// child of it is killed by the operating system, which is what canceling the
// context it was started with means.
func (c *Concurrent) Stop() {
	c.stopped.Store(true)
	if c.stop != nil {
		c.stop()
	}
}

// StartConcurrent runs a command beside the shell and keeps it under its name.
//
// The error is a descriptor the operating system would not give, which is the
// one thing that can fail before the command exists; a name already taken is
// reported as false rather than as an error, because it is the caller's own
// bookkeeping and the caller has a sentence of its own for it.
func (r *Runner) StartConcurrent(ctx context.Context, c ConcurrentCommand) (*Concurrent, bool) {
	if c.Run == nil || c.Name == "" {
		return nil, false
	}
	if _, taken := r.concurrent[c.Name]; taken {
		return nil, false
	}
	// A context of this command's own, so that stopping one does not stop
	// the shell or its neighbors. Derived from the caller's, so a caller that
	// cancels the whole run takes every command beside it along.
	body, stop := context.WithCancel(ctx)
	job := r.startBeside(body, c.Name,
		concurrentStreams{in: c.In, out: c.Out, errs: c.Err, closeEnds: c.CloseEnds}, false, c.Run)
	live := &Concurrent{name: c.Name, job: job, stop: stop}
	if r.concurrent == nil {
		r.concurrent = map[string]*Concurrent{}
	}
	r.concurrent[c.Name] = live
	return live, true
}

// ConcurrentNamed finds a command this runner started, or one a shell above it
// started and it inherited.
func (r *Runner) ConcurrentNamed(name string) (*Concurrent, bool) {
	c, ok := r.concurrent[name]
	return c, ok
}

// ForgetConcurrent takes a name out of *this* runner's table and says whether
// it was there.
//
// It stops nothing: ending the command is [Concurrent.Stop] and is a separate
// act, because the shell whose table is being edited is not always the shell
// that should be ending the command. See the subshell rows at the top of this
// file, where a delete inside one reaches the process and the parent keeps the
// name.
func (r *Runner) ForgetConcurrent(name string) bool {
	if _, ok := r.concurrent[name]; !ok {
		return false
	}
	delete(r.concurrent, name)
	return true
}

// OpenNearEnd puts the shell's own end of a command running beside it into
// the descriptor table, and answers with the number.
//
// [Runner.OpenDescriptor] with one difference, and it is the difference
// between a descriptor the *script* opened and one the *shell* is holding for
// its own plumbing: this one is excluded from the two places a descriptor is
// handed on. An external command does not inherit it, and a shell started
// beside this one does not take a copy of it — which is the exclusion
// `coproc`'s two near ends already have, made reachable because the first
// dialect facility that needed it could not spell it.
//
// **Both halves of that exclusion are load-bearing and the second one was
// measured the hard way.** With a plain [Runner.OpenDescriptor], starting a
// second command beside the shell made the *first* one's output unreadable:
// ownDescriptors duplicates every [os.File] in the table into the new shell,
// so the control end of a pseudo-terminal acquired a second holder, and the
// pair stopped answering. Three commands started in a row and all three read
// back nothing. The entry is the shell's plumbing rather than a file the
// script opened, and saying so is what fixes it.
//
// The file is still the script's to reach by number — `sysread -i $REPLY`,
// `print -u $REPLY` and a redirection all find it — and still the shell's to
// close.
func (r *Runner) OpenNearEnd(f *os.File) int {
	fd := r.nextFreeFd(-1)
	r.setFd(fd, shellOwnedFd{f})
	return fd
}

// concurrentStreams are the three descriptors a command run beside the shell
// runs on. A nil one is the shell's own stream.
type concurrentStreams struct {
	in, out, errs *os.File
	// closeEnds says the command's ends go when the command does. See
	// ConcurrentCommand.CloseEnds for the measurement that makes this a
	// question rather than a rule.
	closeEnds bool
}

// startBeside is the machinery every command run beside the shell shares: a
// job, a subshell of this runner with the caller's descriptors on it, a
// goroutine, and the handing over of what the rest of the shell is waiting on.
//
// It is startCoproc with the pipes lifted out — see coproc.go, which is now
// the pipe-making half and this file's first caller.
func (r *Runner) startBeside(ctx context.Context, name string, s concurrentStreams, asAJob bool, run func(context.Context, *Runner) error) *Job {
	job := &Job{
		done:     make(chan struct{}),
		ready:    make(chan struct{}),
		started:  make(chan struct{}),
		stopNote: make(chan struct{}),
		// And the number a script names it by where the body never reaches a
		// program, which `NAME_PID` reads as `$!` does. See jobident.go.
		ident: r.inventJobIdent(),
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
	// command started beside the shell runs beside it, and a descriptor the
	// script parks and then drops is dropped underneath it otherwise. See
	// ownDescriptors.
	releaseFds := sub.ownDescriptors()
	if s.in != nil {
		sub.Stdin = s.in
	}
	if s.out != nil {
		sub.Stdout = s.out
	}
	// A stream the caller did not name still goes through a lock, as
	// background() and procSub do, and for the reason background() states:
	// the shell carries straight on while this runs, so a lock only this
	// command takes excludes nothing — and a child the shell runs next is
	// copied into the caller's writer by os/exec on a goroutine with no share
	// of it. See #735.
	if s.errs != nil {
		sub.Stderr = s.errs
	} else {
		sub.Stderr = r.lockedStderr()
	}
	r.Stderr, r.Stdout = r.lockedStderr(), r.lockedStdout()
	if !asAJob {
		// Not a job, and so not something to wait for a process id: see
		// Job.settleWithNoProcess.
		job.settleWithNoProcess()
	}
	// Handed over however the goroutine ended, for the reason a background
	// job's status is: the shell waits below for this job to report its
	// process, and a command whose ends stayed open is a shell reading a
	// stream that will never finish. An interpreter bug here has to cost this
	// command and no more.
	status := internalErrorStatus
	r.spawn(func() {
		err := run(ctx, sub)
		// A command run beside the shell is a subshell too, and this is where
		// it ends. Here rather than in each caller's body for the reason
		// endSubshell's own comment gives: a second copy of a boundary's
		// ending is where the next fix goes missing.
		sub.endSubshell(ctx)
		if err != nil {
			sub.diagf("%v\n", err)
		}
		status = sub.status
	}, func() {
		releaseFds()
		job.finish(status)
		// And a fork of this shell has ended: a coprocess is a child in bash
		// and is reaped like one. Measured 2026-09-23, `trap 'echo C' CHLD;
		// coproc CP { echo x; }` fires once for it there and fired not at all
		// here. See Runner.childReaped.
		r.childReapedByTheShell()
		// **Finished first, and then the ends.** Closing them is what turns
		// the command's exit into end-of-file for whoever reads the near end,
		// so in this order a script that has read that end dry *knows* the
		// command has ended — the reaping has something to find, and the
		// notice the next reap point delivers is not a race the script has to
		// win. See retireCoproc, which gates on exactly this, and
		// TestASubshellDeliversTheReapNotice, which is written against the
		// invariant: with the closes first, that test was asserting a reap
		// whose precondition nothing in the script had established, and it
		// lost under load on the runner three times in a day (#2661).
		//
		// Nothing reaches these but this goroutine — ownDescriptors was taken
		// before either was installed, so releaseFds does not hold them — and
		// the shell's own ends are not these, so a `wait` that returns a
		// moment earlier has nothing it can race with here.
		//
		// Each *distinct* file once, and only where the caller asked: a
		// pseudo-terminal hands the same file in as all three of the
		// command's descriptors, and closing it three times would be two
		// closes of a number the operating system may have handed out again
		// in between.
		if s.closeEnds {
			closeOnce(s.in, s.out, s.errs)
		}
	})
	<-job.ready
	<-job.started
	return job
}

// settleWithNoProcess says at once that this command is answered by no process
// of its own, so that starting it does not wait for one.
//
// **A command started through the public seam must come back immediately**,
// and waiting on the pid would not let it. A background job's pid settles when
// the body starts a program, blocks on a read, or completes a pass of a loop
// whose end the shell cannot see — none of which a body made only of builtins
// ever reaches, so `zpty NAME 'print -n hi'` would have blocked until the
// command had finished and `zpty -b NAME 'zselect -t 200'` for two whole
// seconds. Measured by writing the test that way round first.
//
// It costs the process id, which is the honest trade: this is not a job, there
// is no `$!` naming it, and [Concurrent.Ident] is the number the shell
// invented for it. settlePID is one Once, so a program the body starts later
// cannot write a pid behind a reader that has already been told there is none.
func (j *Job) settleWithNoProcess() { j.settleNoPID() }

// closeOnce closes each distinct non-nil file among its arguments.
func closeOnce(files ...*os.File) {
	for i, f := range files {
		if f == nil {
			continue
		}
		seen := false
		for _, earlier := range files[:i] {
			if earlier == f {
				seen = true
				break
			}
		}
		if !seen {
			_ = f.Close()
		}
	}
}
