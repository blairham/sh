// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"slices"
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

	// polled says nothing is blocked on this job's process, so the shell has
	// to ask after it rather than being told. True of what ^Z leaves behind:
	// the command was in the foreground, so no goroutine is waiting on it the
	// way one waits on a `&` job, and once `bg` has let it go there is nobody
	// left to notice it end. See Runner.reapJobs.
	polled bool

	done chan struct{}
	once sync.Once
	// ready is closed once the PID is settled — a real process, or the
	// decision that this job is answered by none. `$!` has to be answerable
	// on the next line, so starting the job cannot return before this is
	// settled.
	ready chan struct{}
	// pidOnce keeps the PID a job's *first* answer and not its latest, and
	// keeps the field's one write on the near side of closing ready. See
	// settlePID.
	pidOnce sync.Once
}

// settlePID records the process this job is answered by, and does it once.
//
// Once is the whole point, and it was a data race before: a job that runs more
// than one external command — `(sleep 0.1; sleep 0.1) &`, a loop, anything
// compound — reached this on each of them, while the shell that started the
// job had already read the field through setLastJob after `<-j.ready` released
// it. The first write is synchronized by that channel and every later one is
// not. `go test -race` finds it; a person finds a `$!` that changed under them.
//
// The first process wins, which is also the better answer. This shell has no
// subshell process to name — a real shell forks and `$!` is the fork — so the
// PID here is an approximation either way, and one that stays put is worth
// more than one that tracks whichever command the job is running now. It is
// what `kill %1` and `jobs -l` are reading.
//
// Writing the field and closing the channel are the *same* Once rather than
// two, which is what makes every reader's `<-j.ready` a real synchronization
// point. They were separate, and the separation was load-bearing in the wrong
// direction: settling a job with no pid could release the shell and leave a
// later process free to write the field behind it. Now a job that has been
// settled without a process stays settled without one — see
// Runner.settleBackgroundJobBeforeABlockingOpen for the case that reaches it,
// and why 0 is the truthful answer there rather than a lost one.
func (j *Job) settlePID(pid int) {
	j.pidOnce.Do(func() {
		j.PID = pid
		close(j.ready)
	})
}

// settleNoPID says this job is answered by no process of its own.
//
// A background builtin or compound command, a job that ended without ever
// reaching an external command, and a job stopped where it stands waiting on
// something outside the shell all arrive here.
func (j *Job) settleNoPID() { j.settlePID(0) }

// settleBackgroundJobBeforeABlockingOpen settles a background job's pid when
// the job is about to open something that may never open.
//
// This is what makes `&` return. Starting a background job waits for the pid
// to settle, because `$!` has to be answerable on the next line — and the two
// places that settled it were a process having started and the job having
// ended. A job whose *first* act is to block reaches neither, so `&` waited
// for an answer that was not coming and the shell never reached the next
// command at all: measured, `(read x < fifo; :) & echo NOW` printed nothing
// here and printed `NOW` at once in all six shells in the panel, which fork
// before they open anything. Every shape of the job did it — a subshell, a
// brace group, a function, a bare builtin, and `sleep 1 < fifo &` too, which
// is an external command that never gets as far as being one.
//
// Opening a fifo with no writer is the shape that reaches it first, because
// redirections are opened before the builtin-or-external dispatch. So the
// point where the job is about to wait on something outside the shell is the
// point where its pid is as settled as it is going to get, and the honest
// answer there is that it has none: nothing has started, and while the job
// stands here nothing will.
//
// Only where the open can really block, and that is the whole reason for the
// stat. Settling before *every* redirection would cost a real answer:
// `sleep 0.3 > log & echo $!` prints the sleep's pid today, and a latch that
// fired on the log file would print 0.
//
// A named pipe and nothing else. A character device was in this test for one
// draft and was a regression rather than a completeness: `/dev/null` is a
// character device, so `sleep 1 > /dev/null & echo $!` — which is about as
// ordinary as a background job gets — started reporting 0. The devices whose
// open really does wait are a terminal claimed by another process group and a
// serial line with no carrier, and neither is reachable from here: a
// background job in this shell is a goroutine in the shell's own process
// group, so it opens the shell's terminal as freely as the shell does. A fifo
// with no peer is the one shape whose open waits by construction, and it is
// the one the panel measurement is about.
//
// A pid that arrives later is dropped rather than recorded, and that is
// deliberate: settlePID is one Once, so the field cannot be written after the
// channel that publishes it has been closed. A job the shell has already
// stopped waiting for cannot be handed a different pid behind the shell's
// back. It is the same trade the doc comment on Job.PID states — a job with
// no process of its own is reported as zero rather than papered over.
func (r *Runner) settleBackgroundJobBeforeABlockingOpen(path string) {
	if r.bg == nil {
		return
	}
	fi, err := os.Stat(path)
	if err != nil {
		// Not there, or not reachable. Either way the open will fail rather
		// than wait, and the job carries on to whatever it does next.
		return
	}
	if fi.Mode()&os.ModeNamedPipe == 0 {
		return
	}
	r.bg.settleNoPID()
}

// settleBackgroundJobBeforeABlockingRead settles a background job's pid when
// the job is about to read a stream that has nothing waiting on it.
//
// The same reasoning as the blocking open above, reached from the other side
// of the same wait. Starting a background job — `&`, `coproc`, or ksh93's `|&`
// — blocks until the job's pid has settled, and the two events that settle it
// are a process having started and the job having ended. A job whose body is a
// builtin that *reads* reaches neither: nothing external runs, and the read
// does not return while the stream stays open. So the shell that started it
// waited for an answer that was never coming, and the hang was total —
// measured, `coproc read x; echo AFTER` printed nothing here and printed
// `AFTER` at once in zsh 5.9.2, bash 5.3.15 and ksh93u+ 2012-08-01, all of
// which fork before the body runs a thing.
//
// A coprocess reaches it every time rather than by arrangement, which is why
// this is a hang a script cannot avoid: its input is a pipe the *shell* holds
// the write end of, so a `read` in its body waits by construction and no
// amount of input on the shell's own stdin ends it. The `&` route needs the
// stream to be one that waits — `{ read x } &` under `/dev/null` finishes at
// once and only hangs when standard input is a pipe with a writer and no data.
//
// Which the axis below has since narrowed rather than removed: a dialect that
// hands a `&` job an empty stream reaches a read that answers at once, so the
// `&` route only arrives here under the answer that hands the job the shell's
// own descriptor. The coprocess route is untouched — its pipe is the shell's
// and no dialect substitutes for it — and so is `wait` on either. See
// backgroundStdin.
//
// The stream is asked rather than assumed, with the poll a shell already has
// for `read -t 0`: a regular file, a here-document and an exhausted stream all
// answer at once, so the job is left alone and `{ read x < file; sleep 1 } &`
// still reports the sleep's pid. Only a stream with nothing waiting on it
// settles the job, and 0 is the truthful answer there for the reason the open
// case gives — nothing has started, and while the job stands here nothing will.
//
// What it does not reach is a body that neither reads nor ever runs a program:
// `{ while :; do :; done } &` still never settles, because there is no point in
// it where the job is waiting on anything outside the shell. That is #1283, and
// it needs a different contract for `$!` rather than a fifth trigger.
func (r *Runner) settleBackgroundJobBeforeABlockingRead(in io.Reader) {
	if r.bg == nil {
		return
	}
	if inputWaiting(in) {
		return
	}
	r.bg.settleNoPID()
}

// settleBackgroundJobAtALoopsBackEdge settles a background job's pid once the
// job has run a whole pass of a loop whose end the shell cannot see.
//
// The fifth and last trigger, and the one the four before it cannot reach.
// Each of those is a point where the job waits on something *outside* the
// shell — a process it started, a fifo with no peer, a stream with nothing on
// it — or where it has ended. A body that only runs builtins in a loop reaches
// none of them, so `&` waited for an answer that was never coming and the
// shell never ran the next command at all: measured 2026-09-07,
// `{ while :; do :; done } & echo AFTER` printed nothing here and printed
// `AFTER` at once in every shell in the panel — dash, bash 5.3.15, that bash
// as `sh`, bash 3.2.57, ksh93u+ and zsh 5.9.2 — all of which fork before the
// body runs a thing. The issue's own repro,
// `{ while :; do :; done } & sleep 0.2; kill %1; echo reached`, never reached.
//
// The back edge and not the start of the body, which is the whole of why this
// costs less than the contract #1283 considered. Settling the moment a job
// begins would make `{ echo hi; sleep 1 } & echo $!` print 0 where it prints
// the sleep's pid today — a real answer lost on the most ordinary shape there
// is. A pass that has *completed* says something narrower and truer: the job
// has already run a whole round of a loop without starting a process, and the
// shell has no way to learn whether it ever will.
//
// Only the loops whose end the shell cannot see, which is what keeps the
// ordinary shapes intact. A `for` over a word list and a `repeat` count know
// how many passes they have before the first one, and a `select` reads on
// every pass and so is already the trigger above; only `while`, `until` and
// `for ((;;))` can run forever on nothing at all. So
// `{ for i in 1 2 3; do :; done; sleep 1 } & echo $!` still prints the sleep's
// pid, and it is `{ i=0; while [ $i -lt 3 ]; do i=$((i+1)); done; sleep 1 } &`
// that now prints 0 — the same trade, paid on the rarer shape.
//
// A pid that arrives later is dropped rather than recorded, exactly as it is
// in the two triggers above: settlePID is one Once, so a job the shell has
// stopped waiting for cannot be handed a different pid behind its back. Which
// is also why a loop whose *first* pass starts a program keeps that program's
// pid — `{ while :; do sleep 1; done } & echo $!` settles on the sleep before
// this is ever reached.
func (r *Runner) settleBackgroundJobAtALoopsBackEdge() {
	if r.bg == nil {
		return
	}
	r.bg.settleNoPID()
}

// backgroundStdin is the standard input a job started with `&` reads.
//
// POSIX XCU 2.9.3 says a background command's standard input "shall be
// assigned to an empty file or /dev/null" while job control is disabled, and
// five of the six columns do exactly that: measured 2026-09-07,
// `<shell> -c '/bin/cat & wait; echo ---; /bin/cat' < f` writes `---` and then
// the file's line in dash, bash 5.3.15, bash-as-`sh`, bash 3.2.57 and ksh93u+,
// and `ls -l /dev/fd/0` inside the job names `/dev/null` by its rdev. zsh alone
// hands the job the shell's own descriptor, and there the line comes out
// *before* the marker because the job ate it.
//
// Which is why the empty stream is the fix and not a nicety. The two readers
// share one descriptor, so this is the direction that loses data rather than
// merely disagreeing about it, and the shape a script writes is the loop:
//
//	while read -r line; do process "$line" & done < input.txt
//
// Anything `process` reads is a line the loop never sees, at status 0, with
// nothing said.
//
// An empty io.Reader rather than an opened `/dev/null`, because a file opened
// here has no owner to close it — the job outlives the statement that started
// it — and the two are indistinguishable to a reader: os/exec hands a child
// the read end of a pipe whose writer sees EOF at once, so an external command
// in the job reads end-of-file exactly as it would from the device. It is also
// what a nil Stdin already means in this package, so the job's input is the
// same kind of empty as a runner that was never given one.
//
// A closed descriptor is the sub-answer, and one column keeps it closed:
// `exec 0<&-; /bin/cat & wait` is silent at 0 in dash and bash and
// `cat: stdin: Bad file descriptor` in ksh93u+ and zsh. It is read through the
// same field rather than a second one — it is the same decision asked of an
// input that is not there — and the marker it turns on is the one
// lockedStdin already passes through untouched, so the job is handed the
// closed descriptor rather than a stand-in for it.
//
// Only while job control is off. That is the condition XCU 2.9.3 states, and
// it is measured rather than inherited: on a pty, `bash -i -c '/bin/cat &
// sleep 0.3; jobs'` reports the job `Stopped` and zsh reports it `suspended
// (tty input)`. Both handed it the *terminal* and let the kernel stop it with
// SIGTTIN — which is an answer an empty input can never produce, so a shell
// with someone to tell must not substitute one.
//
// A redirection on the job still wins, because it is opened inside the job's
// own runner after this: `cat < f &` reads `f` in every dialect, and only the
// *inherited* descriptor is the one the shells disagree about.
func (r *Runner) backgroundStdin() io.Reader {
	in := r.lockedStdin()
	if r.JobControl {
		return in
	}
	switch r.sem().BackgroundJobInput {
	case BackgroundJobInputIsTheShells:
		return in
	case BackgroundJobInputEmptyUnlessClosed:
		if _, closed := in.(closedFd); closed {
			return in
		}
	}
	return emptyReader{}
}

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
	j.settleNoPID()
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
	sub.inheritJobs(jobBoundaryBackground)
	// A background job keeps the parent's trap listing in one shell fewer
	// than a pipeline element does, so it is its own kind of boundary.
	sub.retagTrapBoundary(trapContextBackground)
	sub.bg = job
	// A background job runs concurrently with everything after it, so it
	// shares the caller's streams with the foreground. That is the pipeline
	// race again in a second place: a real shell hands each side a file
	// descriptor and the kernel serializes them, and an io.Writer carries no
	// such guarantee. The shell creates the concurrency, so it guards them.
	sub.Stdout = r.lockedStdout()
	sub.Stderr = r.lockedStderr()
	// Both sides, or neither. A lock one party takes and the other does not
	// excludes nothing, and the shell that started the job is the other
	// party: it carries straight on to the next command while the job runs.
	// Guarding only the job left the parent writing the caller's io.Writer
	// raw, which the race detector found on the trap case where a handler
	// runs on the shell while the job that signaled it is still alive —
	// though `sleep 1 & echo hi` is the same shape without any of that.
	//
	// It stays wrapped afterwards rather than being restored the way a
	// pipeline restores it, because there is nothing to restore it at: the
	// job outlives the statement that started it, and the wrapper delegates,
	// so a stream that is still guarded once the job has gone costs a mutex
	// nobody contends.
	r.Stdout, r.Stderr = sub.Stdout, sub.Stderr
	// And its input, which is where the shells part company: five of the six
	// hand a `&` job an empty stream and zsh hands it the shell's own. See
	// backgroundStdin, and Semantics.BackgroundJobInput.
	//
	// Where the shell's input *is* handed over it is guarded, for the same
	// reason in the other direction: a background job and whatever runs next
	// both read the same stream, and os/exec copies from a caller's io.Reader
	// on a goroutine of its own.
	sub.Stdin = r.backgroundStdin()
	// The job is finished however the goroutine ended, which is what keeps an
	// interpreter bug on it from costing more than the job. The shell is
	// blocked on <-job.ready below and `wait` blocks on the same job
	// afterwards, so a goroutine that stopped without saying so leaves a
	// shell waiting for something that is never coming — a hang where there
	// was a crash, which is the worse of the two.
	//
	// finish marks it ready as well, and both are idempotent: a job that
	// never started a process — a builtin, a compound command — becomes
	// ready when it finishes, with a PID of zero.
	status := internalErrorStatus
	r.spawn(func() {
		// Errors inside a background job are reported where the job runs;
		// there is nowhere to return them to.
		if err := sub.expr(ctx, st.Expr); err != nil {
			sub.diagf("%v\n", err)
			status = 1
			return
		}
		status = sub.status
	}, func() { job.finish(status) })

	// Wait for the PID to be known before returning, so `$!` on the next line
	// is not racing the goroutine that sets it.
	<-job.ready

	// A disowned job — `cmd &!` — is started and then let go of, so it never
	// reaches the table: nothing lists it, `fg` cannot name it, and the next
	// ordinary background job is `[1]` rather than `[2]`, which is measured.
	//
	// Everything else about it is an ordinary background job, and that is
	// measured too: `$!` is still its process, and the shell exits without
	// waiting for it — exactly as it does for `sleep 0.5 &`.
	//
	// It is not announced either, and that one is reasoned rather than
	// measured: an announcement is `[n] pid`, and a job outside the table
	// has no `n` to print. The measurement that would settle it needs an
	// interactive shell with job control, which a script cannot have —
	// `set -m` is `can't change option: -m` in a non-interactive zsh.
	if !st.Disown {
		r.jobs = append(r.jobs, job)
	}
	// `$!` is the most recent background job, which is how a script waits for
	// a specific one — and a disowned job is still the most recent one:
	// measured, two `$!` readings either side of a `&!` differ.
	r.setLastJob(job)
	if !st.Disown {
		r.announceJob(job)
	}
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
	// Before the notices are built rather than after: a job `bg` let go of
	// finishes only when the shell asks, and asking afterwards would report it
	// one prompt later than it ended.
	r.reapJobs()
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

// waitFor blocks for one job, or gives up on it because a trapped signal
// arrived, and reports which of the two happened.
//
// The handler is *not* run here. It runs where every other handler does, at
// the top of the next statement, which is what keeps `wait; echo $?` printing
// the trap's output first and the signal's status second in that order.
func (r *Runner) waitFor(j *Job) (status int, sig syscall.Signal, interrupted bool) {
	// A job nothing is waiting on has to be waited for here, or this would
	// block on a channel no goroutine is ever going to close. That is what ^Z
	// leaves behind — see reapJobs — and between commands this shell is the
	// only waiter, so blocking on the process is not racing anything.
	r.waitOutPolledJob(j)
	if sig, hit := r.awaitOrTrap(j.done); hit {
		return 0, sig, true
	}
	return j.Status, 0, false
}

// waitOutPolledJob blocks on the process of a job the shell has been asking
// after, until it is no longer running.
func (r *Runner) waitOutPolledJob(j *Job) {
	if !j.polled || j.PID == 0 || j.Finished() || r.WaitForCommand == nil {
		return
	}
	for {
		w, err := r.WaitForCommand(j.PID)
		if err != nil {
			j.Stopped = false
			j.finish(r.status)
			return
		}
		status, stopped := r.waitResult(w)
		if stopped {
			// Stopped again while being waited for, which is where zsh's
			// `wait %1` sits for good. Recorded and waited on again rather
			// than answered, because the job has not ended.
			j.Stopped, j.StopSig = true, int(w.Signal)
			continue
		}
		j.Stopped = false
		j.finish(status)
		return
	}
}

// biWait waits for background jobs.
//
// With no arguments it waits for all of them and reports 0, which is what
// every shell in the panel does regardless of how the jobs exited — unless a
// trapped signal cuts the wait short, which is the one thing that gives a
// bare `wait` a status of its own.
func biWait(r *Runner, _ context.Context, args []string) int {
	args, next, code := r.waitOptions(args)
	if code != 0 {
		return code
	}
	if next {
		return r.waitNext()
	}
	if len(args) == 0 {
		for _, j := range r.jobs {
			if _, sig, hit := r.waitFor(j); hit {
				// The jobs are left alone: the wait did not finish, so a
				// later `wait` still has them to wait for.
				return r.interruptedWaitStatus(sig, false)
			}
		}
		// The jobs stay where they are, because a job a bare `wait` reaped
		// is still owed a notice. Measured 2026-09-05 at a prompt on a
		// pseudo-terminal with `sleep 2 &` and then `wait`, so the wait is
		// really what reaps it: bash 5.3.15, bash 3.2.57, bash 3.2 run as
		// `sh`, dash and ksh93u+ all write the `Done` row afterwards, and a
		// `jobs` listing after that writes nothing — so the notice is what
		// forgets them, exactly as it is everywhere else.
		//
		// Unless there is nobody to tell, in which case the notice will
		// never come and this is the only place they can be let go of: a
		// shell with no job control that held them here would start listing
		// finished jobs a shell without this line never listed.
		if !r.JobControl {
			r.jobs = nil
		}
		return 0
	}
	last := 0
	for _, a := range args {
		if strings.HasPrefix(a, "%") {
			last = r.waitJobSpec(a)
			if r.unspecified {
				return r.status
			}
			continue
		}
		pid, ok := atoi(a)
		if !ok {
			return r.waitBadJob(a)
		}
		found := false
		for _, j := range r.jobs {
			if j.PID == pid {
				st, sig, hit := r.waitFor(j)
				if hit {
					return r.interruptedWaitStatus(sig, true)
				}
				last = st
				found = true
			}
		}
		if !found {
			// A number that is not one of this shell's children. Unanimous
			// on the status — 127, the one a command that is not there
			// reports — and two of the four say so out loud.
			if w := r.diag().WaitNotOurChild; w != "" {
				r.diagf("%s\n", Wording(w, "", pid))
			}
			last = 127
		}
	}
	return last
}

// waitOptions reads the leading options, in the three dialects that have any.
//
// zsh has none: `wait -x` is a job spec there and comes back as a job that
// was not found, which is what this did for everybody. The other three refuse
// an option they do not know, in the words and with the usage line their bad
// options already use.
//
// Ending them at `--` is unanimous. `-n` — wait for whichever job finishes
// first — is one dialect's and implemented; its remaining letters (-f, -p)
// and ksh93's --version are not, and say so rather than being taken as a job
// and reported as missing.
func (r *Runner) waitOptions(args []string) (rest []string, next bool, code int) {
	if len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			return args, false, 0
		}
		if a == "--" {
			return args[1:], false, 0
		}
		if a == "-n" {
			// Asked on the exact word: in the shell with no options at all
			// the same word is a job spec, and the axis below decides that.
			if r.ask(r.sem().WaitNWaitsForTheNextJob, "`wait -n` waiting for the next job to finish") {
				return args[1:], true, 0
			}
			if r.unspecified {
				return nil, false, 2
			}
		}
		if !r.ask(r.sem().WaitReadsOptions, "`wait -x` read as an option rather than as a job") {
			return args, false, 0
		}
		if r.unspecified {
			return nil, false, 2
		}
		// Past -n, `wait` has no options this shell implements, so every
		// letter is either one the dialect has and we lack, or unknown.
		return nil, false, r.refuseOption("wait", a, "")
	}
	return args, false, 0
}

// waitNext is `wait -n`: block until whichever job finishes first and report
// its status, forgetting it the way a plain wait for it would. With nothing
// to wait for the answer is a missing command's 127 and no words at all —
// measured in the one shell with the letter.
func (r *Runner) waitNext() int {
	if len(r.jobs) == 0 {
		return 127
	}
	first := make(chan *Job, len(r.jobs))
	for _, j := range r.jobs {
		go func(j *Job) {
			j.Wait()
			first <- j
		}(j)
	}
	j := <-first
	st := j.Status
	r.Forget(j)
	return st
}

// interruptedWaitStatus is what `wait` reports when a trapped signal cut it
// short. named says the wait had an operand rather than being a bare one.
//
// Two questions, and only the second is new. A bare `wait` reports what a
// command killed by that signal reports, which is an encoding this package
// already asks about: 128 plus the signal in most of the panel and 256 plus
// it in one shell, and that shell answers this the same way — 286 for USR1,
// exactly its answer for a command USR1 killed. A `wait` that names a job
// splits differently, and the axis is asked there.
func (r *Runner) interruptedWaitStatus(sig syscall.Signal, named bool) int {
	if named {
		if r.ask(r.sem().WaitForAJobFailsWhenInterrupted,
			"the status of an interrupted `wait` that names a job") {
			return 1
		}
		if r.unspecified {
			return r.status
		}
	}
	st := r.signalDeathStatus(sig)
	if r.unspecified {
		return r.status
	}
	return st
}

// waitJobSpec waits for the job a `%` spec names.
func (r *Runner) waitJobSpec(spec string) int {
	j, code := r.findJobQuietly(spec)
	switch code {
	case jobFound:
		st, sig, hit := r.waitFor(j)
		if hit {
			// The wait did not finish, so the job is not finished with
			// either and stays in the table for the next one.
			return r.interruptedWaitStatus(sig, true)
		}
		r.Forget(j)
		return st
	case jobSpecAmbiguous:
		r.diagf("%s\n", Wording(r.diag().AmbiguousJobSpec,
			"%[1]s: %[2]s: ambiguous job spec", "wait", strings.TrimPrefix(spec, "%")))
		return orDefault(r.diag().WaitNoSuchJobStatus, 127)
	case jobSpecUnanswered:
		return r.status
	}
	// A spec that names nothing: said and failed, or — in one shell —
	// nothing at all and 0.
	if !r.ask(r.sem().WaitReportsAMissingJob, "`wait` reporting a job spec that names nothing") {
		if r.unspecified {
			return r.status
		}
		return 0
	}
	r.diagf("%s\n", Wording(r.diag().WaitNoSuchJob, "wait: %[1]s: no such job", spec))
	return orDefault(r.diag().WaitNoSuchJobStatus, 127)
}

// waitBadJob is an operand that names neither a process nor a job.
//
// Four wordings and three statuses, and none of them is the substrate's old
// one: bash quotes the word and says it is neither a pid nor a job spec,
// dash calls it an illegal number, ksh93 lists what it would have taken, and
// zsh calls it a job that was not found and reports 127 — the status of a
// command that is not there, which is what it takes the operand to have been.
func (r *Runner) waitBadJob(operand string) int {
	d := r.diag()
	r.diagf("%s\n", Wording(d.WaitBadJob, "wait: %[1]s: not a pid", operand))
	return orDefault(d.WaitBadJobStatus, 2)
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
		Stopped: true,
		StopSig: int(sig),
		Command: strings.Join(argv, " "),
		polled:  true,
		done:    make(chan struct{}),
		ready:   make(chan struct{}),
	}
	// Through settlePID rather than as a field, so that the field's one write
	// is the one the channel publishes. A job stopped in the foreground has
	// its process from the start; there is nothing left to settle later.
	job.settlePID(pid)
	r.jobs = append(r.jobs, job)
	r.setLastJob(job)
	r.announceStopped(job)
}

// announceStopped says the job in front of the shell stopped.
//
// Immediately rather than before the next prompt, which is what separates it
// from the notice a job that *ended* gets: measured, `sleep 5; echo after`
// stopped with ^Z prints the notice and then `after` in every shell in the
// panel, so the notice belongs where the stop happened and not where the
// prompt is drawn.
//
// Only where there is somebody to tell, the same rule announceJob follows: a
// script is told nothing about its jobs by any shell in the panel.
func (r *Runner) announceStopped(j *Job) {
	// A new stop is something the person has not been shown, whether or not
	// an earlier one was — measured, a `jobs` listing followed by a second ^Z
	// makes bash warn about stopped jobs at the next `exit` all over again.
	r.toldOfStoppedJobs, r.tellingOfStoppedJobs = false, false
	if !r.JobControl {
		return
	}
	dg := r.diag()
	if dg.JobStoppedNoticeOnANewLine {
		// The terminal echoed `^Z` where the cursor was and left it there, so
		// two of the four start the notice on a line of its own and the other
		// two write it straight after the echo. The same shape as the newline
		// the prompt writes after a ^C, and measured the same way.
		r.errf("\n")
	}
	i := r.jobNumber(j)
	if w := dg.JobStoppedNotice; w != "" {
		r.errf("%s\n", Wording(w, "", i+1, r.jobMarker(j), r.name(), j.Command))
		return
	}
	// Nothing said otherwise, so the notice is the listing's own row, which is
	// what three of the four print.
	r.errf("%s\n", r.jobLineAs(i, j, true, false))
}

// reapJobs asks after the jobs nothing is waiting on, and is how a job that
// `bg` let go of ever finishes.
//
// A `&` job is waited for by the goroutine that started it, and a job `fg`
// put back in front is waited for by `fg` itself. What ^Z leaves behind has
// neither: the command was in the foreground, the wait for it returned when
// it stopped, and after `bg` there is nobody left to notice it end. Without
// this a resumed job is listed as `Running` for the rest of the session,
// never reports that it finished, and never lets go of its process.
//
// A poll rather than a waiter, because the shell asking is the only arrangement
// with exactly one reaper: this runs between commands, where nothing else in
// this shell is waiting for anything, so there is no race over who collects
// the status.
func (r *Runner) reapJobs() {
	if r.PollCommand == nil {
		return
	}
	for _, j := range r.jobs {
		if !j.polled || j.PID == 0 || j.Finished() {
			continue
		}
		w, changed, err := r.PollCommand(j.PID)
		if err != nil {
			// The process is gone and this shell cannot say with what status —
			// most often because something outside reaped it. Finishing it is
			// the honest answer: keeping it would leave a job in the table
			// that nothing can ever resume or report.
			j.Stopped = false
			j.finish(r.status)
			continue
		}
		if !changed {
			continue
		}
		if w.Stopped {
			j.Stopped, j.StopSig = true, int(w.Signal)
			continue
		}
		status, _ := r.waitResult(w)
		j.Stopped = false
		j.finish(status)
	}
}

// HoldsExitForStoppedJobs reports whether this shell should stay rather than
// exit, because leaving now would abandon a job that is stopped — and says so
// when it does.
//
// Exported because both ways out of a session reach it and only one of them is
// in this package: `exit` is a builtin, and the end of input is the front
// end's. Measured, the two are the same warning in the same words.
//
// Once, and what "once" means is measured rather than assumed. The warning
// itself counts as having been told, so a second `exit` leaves; so does a
// `jobs` listing, which is the shell showing the same thing on purpose; and a
// job stopping afterwards starts the count again. Any other command in between
// does not — bash and zsh both warn again after an `echo`.
func (r *Runner) HoldsExitForStoppedJobs() bool {
	if !r.JobControl || r.toldOfStoppedJobs {
		return false
	}
	stopped := false
	for _, j := range r.jobs {
		if j.Stopped && !j.Finished() {
			stopped = true
			break
		}
	}
	if !stopped {
		return false
	}
	if !r.ask(r.sem().StoppedJobsHoldTheExit, "an exit held back by a stopped job") {
		// Unspecified is left to the caller: the axis has already complained,
		// and a shell that cannot say whether to stay had better leave than
		// refuse to.
		return false
	}
	// Both, and the second is what carries it to the next chunk: `exit` twice
	// leaves, and the end of input twice leaves without any chunk running
	// between the two.
	r.toldOfStoppedJobs, r.tellingOfStoppedJobs = true, true
	// Written plainly rather than through the dialect's location prefix: one
	// of the two shells that says this names itself in the sentence and the
	// other names nobody at all, and neither writes the line number a prompt's
	// diagnostics carry.
	r.errf("%s\n", Wording(r.diag().StoppedJobsAtExit, "there are stopped jobs", r.name()))
	return true
}

// LastCommandWasInterrupted reports whether the command that just ran ended
// because it was interrupted.
//
// For the shell around it, and for one thing only: the terminal echoed `^C`
// where the cursor was and left it there, so the next prompt has to start on a
// line of its own. That is the same debt a stopped job's notice pays, and the
// two are answered in different places because a stop is something this
// package prints and an interrupt is not.
//
// Asked of the command rather than of the process, and that is the whole
// reason it exists. A shell that hands the terminal to what it runs is no
// longer in the foreground group, so the ^C never reaches it and a handler of
// its own hears nothing — which is exactly the arrangement job control puts it
// in. What ended the command is the fact; hearing the signal was only ever a
// proxy for it, and one that stopped being true the moment `fg` and `bg` could
// work at all.
func (r *Runner) LastCommandWasInterrupted() bool { return r.diedOfSig == syscall.SIGINT }

// setLastJob makes a job the current one and records its pid as `$!`.
//
// Two fields written together, and read apart. The job pointer is what `%%`
// and a bare `fg` follow and is dropped when the job leaves the table; the pid
// is `$!` and is never dropped, because no shell in the panel empties it.
func (r *Runner) setLastJob(j *Job) {
	r.lastJob, r.lastJobPID, r.lastJobPIDSet = j, j.PID, true
}

// Jobs is what this shell is keeping track of, oldest first.
//
// For the shell around it: a `jobs` builtin has to list them and `fg` has to
// find one by number, and the bookkeeping is this package's.
//
// A copy of the header rather than the table itself, and that is the whole of
// what it buys: reporting a finished job compacts the table in place and
// clears the slots past the survivors, so that a job nobody lists is not held
// alive by an array nobody reads. A caller still holding the header it was
// handed before that compaction would be holding those cleared slots, and
// `Wait` on a nil job is a segmentation fault rather than an error. It took
// only a prompt drawn between taking the slice and walking it.
//
// The Job pointers are shared on purpose. A caller that took a job while it
// was running can still wait for it after the table has forgotten it, which is
// the question a caller holding a job is asking.
func (r *Runner) Jobs() []*Job { return slices.Clone(r.jobs) }

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
