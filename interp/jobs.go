// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
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
	//
	// It is what has to be *signaled* and what has to be *waited for*, so it
	// stays the kernel's answer and nothing else: a number invented here
	// would be handed to `kill` and to the front end's wait hook. What a
	// script reads is Job.Ident, which falls back to an invented number
	// exactly where this is 0.
	PID    int
	Status int

	// ident is the number a *script* names this job by: `$!`, `wait <n>`,
	// `kill <n>`, and the id a `jobs -l` or `jobs -p` listing prints. It is
	// the process id where the job has one, and a number this shell invented
	// where it does not — see jobident.go for why a job with no process of
	// its own still has to answer to one, and why the invented numbers start
	// where they do.
	//
	// Assigned when the job is built, before anything can run, so that it is
	// settled before `&` returns whether or not a process ever appears.
	ident int

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
	// ownGroup says the process is the leader of a process group of its own,
	// which is what makes it signalable *as a group*. False where the job
	// runs in the shell's own group — a background job started with the
	// monitor off, which is what every shell in the panel does (#1738) — and
	// then a signal aimed at the group would be aimed at the shell.
	//
	// Written inside pidOnce with the PID, for the reason the PID is written
	// there: the two are one fact about one process, and a reader that saw
	// the pid without knowing which kind of target it is would have to guess.
	ownGroup bool

	// num is the number this job is listed under and named by: `%2` is the
	// job whose num is 2, for as long as the job is in the table. Assigned
	// when the job enters it and never reassigned — see Runner.addJob.
	num int

	// procs is every process the job is made of *now*, which is not the same
	// question as PID and is the one `kill %1` asks. See Job.took.
	procsMu sync.Mutex
	procs   []jobProcess

	// parts counts the pieces of the job that have still to start, and
	// started is closed when the count reaches zero. A backgrounded pipeline
	// is more than one process and `&` must not return before all of them
	// exist — see Job.expectPart.
	partsMu sync.Mutex
	parts   int
	started chan struct{}

	// stopNote is closed by the goroutine waiting on this job's process, the
	// first time that process is seen to have *stopped* rather than ended.
	//
	// A channel and not a field, because the two ends are different
	// goroutines: the job's own runs the command and does the waiting, and
	// the shell carries straight on to the next statement. Closing it is the
	// publication, so stopSig may be written plainly on the near side and
	// read plainly on the far side with nothing else synchronizing them.
	//
	// nil is a job nobody could ever say that about — every one built by
	// [Runner.background] and by `coproc` has one — and it is safe rather
	// than merely unused: a receive on a nil channel is never ready, so a
	// job without one is a job that never stopped, in the poll below and in
	// the select `wait` makes over it alike.
	stopNote chan struct{}
	stopOnce sync.Once
	stopSig  syscall.Signal
	// reportedState is the state this job was in the last time the shell
	// said anything about it. `jobs -n` is the only reader: it lists the
	// jobs whose state has moved since, which is a question about what the
	// shell has *told somebody* rather than about the job.
	//
	// The zero value is the running state and that is the right start rather
	// than a convenience: a job is running when it is made, so a shell that
	// has said nothing about it yet has nothing to correct.
	reportedState jobReportState

	// noticedStop says the shell has already taken that note into the job's
	// own Stopped and StopSig. Written only on the shell's own goroutine,
	// which is what keeps `bg` from being undone: a job the script resumed
	// is running again, and a note that fired before it was resumed must not
	// put it back to stopped the next time anything looks.
	noticedStop bool
}

// expectPart says one more piece of this job has still to start.
//
// Counted rather than assumed, because the shell that ran `&` blocks until
// every piece is there. `$!` is one pid and [Job.ready] is what answers it,
// but `kill %1` on the very next line means the whole job — and a pipeline
// element that had not reached its fork yet was a process the kill could not
// name and the shell then waited out in full. Measured against bash 5.3.15:
// `sleep 6 | cat & kill %1; wait` returns at once there, and returned at once
// here only when the race fell the right way (#2295).
//
// A part added after the shell has already been released is dropped rather
// than counted. That is not a lost piece: the shell is past the `&`, so there
// is nothing left to hold, and a count raised behind a closed gate would be a
// count nobody ever lowers.
func (j *Job) expectPart(n int) {
	j.partsMu.Lock()
	defer j.partsMu.Unlock()
	if j.started == nil {
		// A job nobody is waiting on this way — one this shell was told about
		// rather than one it started. There is no gate to hold.
		return
	}
	select {
	case <-j.started:
		return
	default:
	}
	j.parts += n
}

// partStarted lowers the count expectPart raised, and opens the gate at zero.
func (j *Job) partStarted() {
	j.partsMu.Lock()
	defer j.partsMu.Unlock()
	if j.started == nil || j.parts == 0 {
		return
	}
	j.parts--
	if j.parts == 0 {
		close(j.started)
	}
}

// jobPart is one piece of a job, and the once that keeps it from being
// counted as started twice: a pipeline element reaches that point either by
// starting a process, by waiting on something outside the shell, or by
// ending, and whichever comes first is the one that counts.
type jobPart struct {
	job  *Job
	once sync.Once
}

// started says this piece of the job is as started as it is going to get.
func (p *jobPart) started() {
	if p == nil {
		return
	}
	p.once.Do(p.job.partStarted)
}

// noteStopped records that this job's process stopped, and does it once.
//
// Called on the job's own goroutine — the one blocked on the process — and
// read on the shell's. See Job.stopNote for why closing the channel is the
// whole of the synchronization.
func (j *Job) noteStopped(sig syscall.Signal) {
	j.stopOnce.Do(func() {
		j.stopSig = sig
		close(j.stopNote)
	})
}

// sawStop reports the stop the goroutine waiting on this job's process saw,
// if it saw one. It does not block.
func (j *Job) sawStop() (syscall.Signal, bool) {
	select {
	case <-j.stopNote:
		return j.stopSig, true
	default:
		return 0, false
	}
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
	// The job body is a piece of the job like a pipeline element is, and this
	// is the point it has started: a pid settled, or the decision that there
	// is none. A pipeline raises the count for its other elements before any
	// of them runs, so the count cannot reach zero in between.
	j.partStarted()
}

// settleStartedPID is settlePID for a process this shell has just started,
// which is the one caller that knows whether it was given a group of its own.
//
// One write of both fields, inside the same Once: whether the pid names a
// group or a process is as much a fact about it as the number, and a reader
// that had the one without the other would have to guess which signal to
// send. See Job.ownGroup.
func (j *Job) settleStartedPID(pid int, ownGroup bool) {
	j.pidOnce.Do(func() {
		j.PID, j.ownGroup = pid, ownGroup
		close(j.ready)
	})
	j.partStarted()
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
	if r.bg == nil && r.part == nil {
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
	r.settleWaitingJobPart()
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
	if r.bg == nil && r.part == nil {
		return
	}
	if inputWaiting(in) {
		return
	}
	r.settleWaitingJobPart()
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
	if r.bg == nil && r.part == nil {
		return
	}
	r.settleWaitingJobPart()
}

// settleWaitingJobPart is what the three triggers above do once they have
// decided the job is about to wait on something outside the shell: the job's
// pid settles at zero where this shell is the one that names it, and either
// way the piece of the job it is has started as far as it ever will.
//
// Both, because a pipeline element reaches these too and only one element
// names the job. An element blocked on a fifo with no peer is a piece the
// shell that ran `&` would otherwise wait for forever.
func (r *Runner) settleWaitingJobPart() {
	if r.bg != nil {
		r.bg.settleNoPID()
	}
	r.part.started()
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

// jobReportState is the coarse state a listing reports: running, stopped, or
// ended. Only the transitions between these three are what `jobs -n` calls a
// change — a job that is still running is still running however much it has
// done since.
type jobReportState int

const (
	jobRunningState jobReportState = iota
	jobStoppedState
	jobEndedState
)

// reportState is what a listing would say about this job right now.
func (j *Job) reportState() jobReportState {
	switch {
	case j.Finished():
		return jobEndedState
	case j.Stopped:
		return jobStoppedState
	}
	return jobRunningState
}

// Ident is the number a script names this job by.
//
// The process id where the job has one, and the number this shell invented for
// it where it has none. One reader's question — "which number does `$!` give
// for this job, and which number does `wait` take back" — asked in one place,
// so that a caller cannot answer half of it: `$!`, `wait <n>`, `kill <n>`, the
// `[n] pid` announcement and a `jobs -l` row all read this.
//
// Read after the job's pid has settled, which every caller is: starting a job
// blocks on Job.ready, and the field PID is written inside the same Once that
// closes it.
func (j *Job) Ident() int {
	if j.PID != 0 {
		return j.PID
	}
	return j.ident
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
		done:     make(chan struct{}),
		ready:    make(chan struct{}),
		started:  make(chan struct{}),
		stopNote: make(chan struct{}),
		// The number a script will name it by if no process ever answers for
		// it. Taken here rather than where the pid settles, because that
		// happens on the job's own goroutine and `$!` is read on the shell's
		// — and because a job the shell never gets a process for must still
		// have an answer by the time `&` returns. See jobident.go.
		ident: r.inventJobIdent(),
		// The job body itself, released when its pid settles either way. It
		// is raised here rather than inside the goroutine so that nothing can
		// read the count before it is there.
		parts: 1,
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
	// The fork a background job is, which a `( … )` standing as its body does
	// not fork again: measured 2026-09-17, ksh93u+'s `a=(1 2 3); ( unset
	// "a[1]"; echo "[${a[*]}]" ) &` prints `[1 3]` where the same subshell in
	// the foreground prints `[]`, and `{ ( … ); } &` agrees. So the
	// parentheses there are the fork itself and their arrays are its own
	// copy. See Runner.unsetEmptiesAnUnwrittenArray.
	sub.forkedForABackgroundJob = true
	// A background job outlives the shell that started it, so where that
	// shell is the body of a process substitution the job keeps the
	// substitution's end of the pipe open — the copy of the descriptor a
	// real shell's fork hands it, reconstructed. Released when the job
	// finishes, however it finished. See substEnd for the measurement.
	releasePipeEnd := sub.holdPipeEnd()
	// And its own copy of the descriptor table, for the reason the hold
	// above exists in the other direction: the shell carries straight on
	// while the job runs, so a descriptor the script drops afterwards would
	// otherwise be dropped underneath it. See ownDescriptors.
	releaseFds := sub.ownDescriptors()
	// And the group a fork would have given the job. Its lifetime is the
	// job's, not the statement's: `{ … } &` returns at once and the body goes
	// on running, so the release goes with the two above rather than here.
	// See Runner.anchorForkedBody.
	releaseAnchor := sub.anchorForkedBody()
	// A background job keeps the parent's trap listing in one shell fewer
	// than a pipeline element does, so it is its own kind of boundary.
	sub.retagTrapBoundary(trapContextBackground)
	sub.bg = job
	// Every process this shell and anything it clones starts is a process of
	// this job. Wider than bg on purpose: a pipeline clears bg on all but its
	// last element so that one pid is settled once, and inJob is what keeps
	// the other elements attached to the job they are part of.
	sub.inJob = job
	// And a job nested inside this one is its own, so it does not answer for
	// a piece of the job around it.
	sub.part = nil
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
		// The job is a subshell, and it is over: its own EXIT trap runs
		// here, before the status the shell will report for it is read.
		sub.endSubshell(ctx)
		status = sub.status
	}, func() {
		job.finish(status)
		// After the job is finished rather than before it, so nothing can
		// observe a pipe that has ended while the job that was writing to
		// it is still marked as running.
		releasePipeEnd()
		releaseFds()
		releaseAnchor()
	})

	// Wait for the PID to be known before returning, so `$!` on the next line
	// is not racing the goroutine that sets it.
	<-job.ready
	// And for every other piece of the job to have started, so `kill %1` on
	// that same next line is not racing them either. One pipeline element
	// settles the pid and the rest are pieces this gate counts — see
	// Job.expectPart.
	<-job.started

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
		r.addJob(job)
	}
	// `$!` is the most recent background job, which is how a script waits for
	// a specific one — and a disowned job is still the most recent one:
	// measured, two `$!` readings either side of a `&!` differ.
	r.setLastJob(job)
	if !st.Disown {
		// The markers are the other half, and a disowned job is in neither
		// the table nor this list: `%%` names a job something can still
		// reach.
		r.becomeCurrentJob(job)
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
	if !r.canAnnounce() {
		return
	}
	if !r.ask(r.sem().AnnouncesBackgroundJob, "a background job being announced") {
		return
	}
	if !r.monitor &&
		!r.ask(r.sem().AnnouncesBackgroundJobWithoutTheMonitor,
			"a background job being announced with the monitor off") {
		// The monitor is off and this dialect stops announcing with it. Two
		// of the panel carry on and two go quiet, which is why it is asked
		// rather than assumed either way — see the axis (#1738).
		return
	}
	r.errf("%s\n", Wording(r.diag().JobStarted, "[%[1]d] %[2]d", job.num, job.Ident()))
}

// canAnnounce reports whether this shell has anybody to tell that a job
// started.
//
// A prompt is that somebody in every dialect, and one dialect also counts the
// monitor on its own — see Semantics.MonitorAloneAnnouncesAJob, which is the
// same seam canResume reads one notice over and which the panel divides
// differently. Read and not asked, for the reason the axis gives.
func (r *Runner) canAnnounce() bool {
	return r.JobControl || (r.monitor && r.sem().MonitorAloneAnnouncesAJob == Yes)
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
	if !r.monitor {
		// Shared ground rather than an axis: measured 2026-09-10 on a
		// pseudo-terminal with the monitor off, no shell in the panel says
		// anything when a background job finishes — not even the two that
		// still announce its *start*. dash's late report of one with an empty
		// command is its own oddity, measured and not reproduced (#1738).
		return nil
	}
	// Before the notices are built rather than after: a job `bg` let go of
	// finishes only when the shell asks, and asking afterwards would report it
	// one prompt later than it ended.
	r.reapJobs()
	var lines []string
	kept := r.jobs[:0]
	for _, j := range r.jobs {
		if !j.Finished() {
			kept = append(kept, j)
			continue
		}
		// The command is always shown, even in the two dialects that leave
		// it out of a `jobs` listing: both of them print it here. That is
		// what makes JobsShowBackgroundCommand a question about the listing
		// rather than about the text.
		lines = append(lines, r.jobLineAs(j.num, j, true, true))
	}
	for i := len(kept); i < len(r.jobs); i++ {
		r.jobs[i] = nil
	}
	r.jobs = kept
	return lines
}

// waitFor blocks for one job, or gives up on it because a trapped signal
// arrived, and reports which of the two happened.
//
// The handler is *not* run here. It runs where every other handler does, at
// the top of the next statement, which is what keeps `wait; echo $?` printing
// the trap's output first and the signal's status second in that order.
func (r *Runner) waitFor(j *Job) (status int, sig syscall.Signal, interrupted, stopped bool) {
	// A job nothing is waiting on has to be waited for here, or this would
	// block on a channel no goroutine is ever going to close. That is what ^Z
	// leaves behind — see reapJobs — and between commands this shell is the
	// only waiter, so blocking on the process is not racing anything.
	r.waitOutPolledJob(j)
	giveUp := r.stoppedJobEndsAWait(j)
	if r.unspecified {
		// The axis went unanswered and this shell has said so. Waiting after
		// that would be picking one of the two answers anyway — and the one
		// that can wait for ever.
		return 0, 0, false, false
	}
	if giveUp != nil && r.noticeStoppedJob(j) {
		// Already standing stopped when the wait was asked for, which is the
		// ordinary shape: a script stops a job and then waits for it.
		return 0, 0, false, true
	}
	sig, hit, gaveUp := r.awaitOrTrap(j.done, giveUp)
	if hit {
		return 0, sig, true, false
	}
	if gaveUp {
		r.noticeStoppedJob(j)
		return 0, 0, false, true
	}
	return j.Status, 0, false, false
}

// stoppedJobEndsAWait is what ends a wait for this job because the job
// stopped, or nil where this shell goes on waiting for it.
//
// nil in three cases and each is a different reason. The dialect may simply
// wait — the base answer, and what four of the five columns do. The monitor
// may be off, which is where the panel is unanimous about waiting and where
// a `&` job does not even have a process group of its own to be stopped as.
// And a job with no process of its own — a background builtin, a compound
// command — has nothing that can stop, so there is nothing to be told.
func (r *Runner) stoppedJobEndsAWait(j *Job) <-chan struct{} {
	if !r.monitor || j.PID == 0 {
		return nil
	}
	if !r.ask(r.sem().WaitGivesUpOnAStoppedJob, "a `wait` given up because the job stopped") {
		return nil
	}
	return j.stopNote
}

// stoppedWaitStatus is what a `wait` that named a job reports when it gave up
// because the job stopped: the status of a command that signal killed, which
// is what bash answers — 145 for the SIGSTOP it was measured with.
func (r *Runner) stoppedWaitStatus(j *Job) int {
	return r.signalDeathStatus(syscall.Signal(j.StopSig))
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

// noticeWaitedSignal says out loud that the child this `wait` reaped was ended
// by a signal, in the one dialect that does.
//
// Only a `wait` that *named* something. A bare `wait` says nothing in every
// column, measured, which is why this is called from the two narrowed routes
// and not from the loop over every job.
//
// The signal is read back out of the status rather than carried on the Job,
// and that is exact rather than a guess in the only dialect that reaches here:
// its encoding is 256 plus the signal (Semantics.SignalDeathStatusIsTwoFiftySix)
// and no `exit` can produce a status above 255, so nothing but a signal death
// lands in that range. A dialect encoding with 128 would be ambiguous — 143 is
// both `exit 143` and a TERM — and none of those has a wording here, so the
// ambiguity is not reachable. If one ever gains one, the signal wants carrying
// on the Job instead.
func (r *Runner) noticeWaitedSignal(j *Job, status int) {
	w := r.diag().WaitSignalNotice
	if w == "" {
		return
	}
	base := r.signalDeathStatus(0)
	if r.unspecified {
		return
	}
	sig := status - base
	if sig <= 0 || sig > maxNamedSignal {
		return
	}
	r.diagf("%s\n", Wording(w, "wait: %[1]d: %[2]s", j.Ident(), r.signalDescription(syscall.Signal(sig))))
}

// maxNamedSignal bounds what a status may be read back as a signal. Higher
// than any signal either supported platform has, and low enough that an
// ordinary status cannot reach it once the 256 base is taken off.
const maxNamedSignal = 64

// biWait waits for background jobs.
//
// With no arguments it waits for all of them and reports 0, which is what
// every shell in the panel does regardless of how the jobs exited — unless a
// trapped signal cuts the wait short, which is the one thing that gives a
// bare `wait` a status of its own.
func biWait(r *Runner, _ context.Context, args []string) int {
	// Whatever this wait is for, the shell is about to reap what it can —
	// which is where a coprocess that ended is noticed. See
	// Runner.retireCoproc for the measurements, and note that a `wait` is one
	// of the ways rather than the way: a subshell or an external command
	// delivers the same notice without any wait being written.
	defer r.retireCoproc()
	args, opts, code := r.waitOptions(args)
	if code != 0 {
		return code
	}
	if opts.next {
		if opts.reading == WaitNextJobFirstToSucceed {
			if len(args) == 0 {
				return r.waitUntilOneSucceeds(opts)
			}
			// With operands the letter changes nothing, measured: `wait -n
			// p1 p2` waits both out and reports the last one's status,
			// exactly as `wait p1 p2` does. So this falls through to the
			// ordinary operand walk below rather than to waitNext.
			opts.next = false
		} else {
			return r.waitNext(args, opts)
		}
	}
	if len(args) == 0 {
		// A bare `wait` names no job, so `-p` empties its variable rather
		// than leaving it alone — measured, and the same rule the narrowed
		// form follows when it finds nothing to wait for.
		r.storeWaitedPID(opts, nil)
		for _, j := range r.jobs {
			_, sig, hit, stopped := r.waitFor(j)
			if r.unspecified {
				return r.status
			}
			if hit {
				// The jobs are left alone: the wait did not finish, so a
				// later `wait` still has them to wait for.
				return r.interruptedWaitStatus(sig, false)
			}
			if stopped {
				// Said and stepped over rather than returned on: a bare
				// `wait` is for every job, and giving up on one of them is
				// not giving up on the rest. Measured — with a stopped job
				// and a running one, bash warns and then waits out the
				// running one before coming back at 0.
				r.reportStoppedWait(j, r.diag().WaitJobStopped, j.Ident())
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
			r.jobs, r.jobOrder = nil, nil
		}
		return 0
	}
	last := 0
	// The last job actually waited out, which is what `-p` names when the
	// operands are several: measured, `wait -p V $b1 $b2` leaves V holding
	// b2 — the last id, and the one whose status is reported.
	var named *Job
	defer func() { r.storeWaitedPID(opts, named) }()
	for _, a := range args {
		if strings.HasPrefix(a, "%") {
			var j *Job
			last, j = r.waitJobSpecNaming(a)
			if j != nil {
				named = j
			}
			if r.unspecified {
				return r.status
			}
			continue
		}
		pid, ok := atoi(a)
		if !ok {
			return r.waitBadJob(a)
		}
		waitable := r.waitableByIdent(pid)
		if r.unspecified {
			// The axis above went unanswered, so this shell has refused
			// rather than reported. Saying `no such process` on top of that
			// would be answering it after all.
			return r.status
		}
		found := false
		for _, j := range waitable {
			st, sig, hit, stopped := r.waitFor(j)
			if r.unspecified {
				return r.status
			}
			if hit {
				return r.interruptedWaitStatus(sig, true)
			}
			if stopped {
				// Nothing said on this route — measured, `wait $!` for a
				// stopped job is 145 in silence where `wait %1` warns —
				// and the job stays in the table, because a job that
				// stopped is not a job that finished.
				return r.stoppedWaitStatus(j)
			}
			last, named = st, j
			found = true
			r.noticeWaitedSignal(j, st)
			// And the job is finished with, exactly as the `%` spec route
			// finishes with the job it waited out: its number goes back to
			// the table. See Runner.reap.
			r.reap(j)
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
func (r *Runner) waitOptions(args []string) (rest []string, opts waitOpts, code int) {
	for len(args) > 0 {
		a := args[0]
		if len(a) < 2 || a[0] != '-' {
			return args, opts, 0
		}
		if a == "--" {
			return args[1:], opts, 0
		}
		if a == "-n" {
			// Asked on the exact word: in the shell with no options at all
			// the same word is a job spec, and the axis below decides that.
			if reading := r.waitNextJob(); reading != WaitNextJobAbsent {
				args, opts.next, opts.reading = args[1:], true, reading
				continue
			}
			if r.unspecified {
				return nil, waitOpts{}, 2
			}
		}
		if !r.ask(r.sem().WaitReadsOptions, "`wait -x` read as an option rather than as a job") {
			return args, opts, 0
		}
		if r.unspecified {
			return nil, waitOpts{}, 2
		}
		// Past -n there is one more letter this shell implements, and it
		// takes an argument — so the word has to be walked rather than
		// compared, and a bundle spends the rest of itself on that argument
		// the way every other option word here does: `-np V` and `-npV` are
		// both the pair. Measured 2026-09-13 on bash 5.3.15.
		used, code := r.waitLetters(a, args[1:], &opts)
		if code != 0 {
			return nil, waitOpts{}, code
		}
		args = args[1+used:]
	}
	return args, opts, 0
}

// waitOpts is what the option words asked for: `-n` and `-p var`.
//
// A struct rather than two results because the pair is read together and
// `wait -p V -n %2` is one request — and because the next letter this shell
// grows will be a third thing the same walk collects.
type waitOpts struct {
	// next is `-n`: report the first job to finish rather than waiting the
	// operands out in order.
	next bool
	// reading is which of the two shapes of `-n` this dialect has, carried
	// beside the flag so the waiting code does not ask the axis a second
	// time. See Semantics.WaitNextJob.
	reading WaitNextJobReading
	// pvar is `-p`'s argument, the name the finished job's process id is
	// stored under, and named says the letter was given at all. The two are
	// separate because the store happens even when there is no job to name:
	// measured, a bare `wait -p V` empties V rather than leaving it alone.
	pvar  string
	named bool
}

// waitLetters walks one option word past the leading `-`, and returns how
// many of the words *after* it were consumed as an argument.
//
// A letter this shell does not implement is refused whole-word, which is what
// the panel does: the diagnostic names the word rather than the letter inside
// it, and Diagnostics.UnimplementedOptionLetters is what separates a letter
// the dialect has from one nobody has.
func (r *Runner) waitLetters(word string, rest []string, opts *waitOpts) (used, code int) {
	letters := word[1:]
	for i := 0; i < len(letters); i++ {
		switch letters[i] {
		case 'n':
			reading := r.waitNextJob()
			if reading == WaitNextJobAbsent {
				if r.unspecified {
					return 0, 2
				}
				return 0, r.refuseOption("wait", word, "")
			}
			opts.next, opts.reading = true, reading
		case 'p':
			if !r.ask(r.sem().WaitPNamesTheFinishedJob, "`wait -p var` naming the job the status came from") {
				if r.unspecified {
					return 0, 2
				}
				return 0, r.refuseOption("wait", word, "")
			}
			opts.named = true
			if tail := letters[i+1:]; tail != "" {
				opts.pvar = tail
				return 0, 0
			}
			if len(rest) == 0 {
				// The letter is there and its argument is not. bash's own
				// wording for an option that wants one.
				return 0, r.refuseOption("wait", word, "")
			}
			opts.pvar = rest[0]
			return 1, 0
		default:
			return 0, r.refuseOption("wait", word, "")
		}
	}
	return 0, 0
}

// waitNext is `wait -n`: block until whichever job finishes first and report
// its status, forgetting it the way a plain wait for it would. With nothing
// to wait for the answer is a missing command's 127 and no words at all —
// measured in the one shell with the letter.
//
// The operands narrow which jobs count. `wait -n` with nothing after it takes
// the first of every job the shell holds; `wait -n %2 %3` takes the first of
// those two and lets a job that finishes sooner go on being a job. Measured
// 2026-09-13 on bash 5.3.15 with a one-second job and a two-second one: the
// bare form reports the first and the narrowed form reports the second.
//
// This function used to take no arguments at all, which is the whole of the
// defect: a script that named the jobs it cared about was answered by
// whichever unrelated job happened to end first, seconds too early and with
// another job's status.
// waitNextJob resolves the axis, and only where a `-n` word was really
// written. An unanswered one refuses the way every other axis does.
func (r *Runner) waitNextJob() WaitNextJobReading {
	reading := r.sem().WaitNextJob
	if reading == WaitNextJobUnspecified {
		r.diagf("%s\n", r.unanswered("`wait -n`"))
		r.status = 2
		r.unspecified = true
		return WaitNextJobAbsent
	}
	return reading
}

// waitUntilOneSucceeds is the second reading of `wait -n` with no operands:
// the jobs are waited out in order and the first one that exited 0 ends the
// wait at 0, while running out without one reports 129.
//
// See Semantics.WaitNextJobFirstToSucceed for the four arrangements that tell
// this from the reading next door, from a plain `wait`, and from a refusal.
// The jobs that were not reached are left where they are, exactly as a bare
// `wait` leaves the ones it reaped: a job still owed a notice is still owed
// one.
func (r *Runner) waitUntilOneSucceeds(opts waitOpts) int {
	r.storeWaitedPID(opts, nil)
	// The jobs that were still running when this wait *began*, taken as a
	// snapshot rather than asked per job as the loop goes.
	//
	// A job that had already ended does not count: measured, with `sh -c
	// 'exit 7' &` and a second in between, `wait -n` is 0 there, and 129 for
	// the same job when it is still running. So the 129 is about a job this
	// wait really waited out, which is what makes a shell that has already
	// reaped everything answer 0 rather than "none succeeded".
	//
	// Asked once at the top because the loop takes time: a job that was
	// running when the wait began and finished while an earlier one was
	// being waited out is still one of this wait's jobs, and asking it again
	// halfway through would drop it. That is the second row of the
	// measurement — two jobs, the failing one finishing first — where the
	// answer is the later job's success.
	var live []*Job
	for _, j := range r.jobs {
		if !j.Finished() {
			live = append(live, j)
		}
	}
	waited := false
	for _, j := range live {
		status, sig, hit, stopped := r.waitFor(j)
		if r.unspecified {
			return r.status
		}
		if hit {
			return r.interruptedWaitStatus(sig, false)
		}
		if stopped {
			// Said and stepped over, as a bare `wait` does: giving up on one
			// job is not giving up on the rest.
			r.reportStoppedWait(j, r.diag().WaitJobStopped, j.Ident())
			continue
		}
		if status == 0 {
			return 0
		}
		waited = true
	}
	if !waited {
		// Nothing was waited out, so nothing failed to succeed: measured,
		// `wait -n` with no jobs at all is 0 there, where the other reading
		// answers 127. The 129 is what a run that *did* wait and found no
		// success reports.
		return 0
	}
	return waitNextJobNoneSucceeded
}

func (r *Runner) waitNext(args []string, opts waitOpts) int {
	jobs, code := r.waitNextJobs(args)
	if code != 0 {
		r.storeWaitedPID(opts, nil)
		return code
	}
	if len(jobs) == 0 {
		r.storeWaitedPID(opts, nil)
		return 127
	}
	first := make(chan *Job, len(jobs))
	for _, j := range jobs {
		go func(j *Job) {
			j.Wait()
			first <- j
		}(j)
	}
	j := <-first
	st := j.Status
	r.storeWaitedPID(opts, j)
	r.reap(j)
	return st
}

// waitNextJobs is the set `wait -n` may return from: every job with no
// operands, and the ones the operands name otherwise.
//
// An operand that names nothing is the same complaint and the same status a
// plain `wait` gives it, which is what keeps one spelling of "no such job"
// in the shell rather than two.
func (r *Runner) waitNextJobs(args []string) ([]*Job, int) {
	if len(args) == 0 {
		return r.jobs, 0
	}
	var jobs []*Job
	for _, a := range args {
		if strings.HasPrefix(a, "%") {
			j, code := r.findJobQuietly(a)
			if code == jobFound {
				jobs = append(jobs, j)
				continue
			}
			if code == jobSpecAmbiguous {
				r.diagf("%s\n", Wording(r.diag().AmbiguousJobSpec,
					"%[1]s: %[2]s: ambiguous job spec", "wait", strings.TrimPrefix(a, "%")))
				return nil, orDefault(r.diag().WaitNoSuchJobStatus, 127)
			}
			if code == jobSpecUnanswered {
				return nil, r.status
			}
			if !r.ask(r.sem().WaitReportsAMissingJob, "`wait` reporting a job spec that names nothing") {
				if r.unspecified {
					return nil, r.status
				}
				continue
			}
			r.diagf("%s\n", Wording(r.diag().WaitNoSuchJob, "wait: %[1]s: no such job", a))
			return nil, orDefault(r.diag().WaitNoSuchJobStatus, 127)
		}
		pid, ok := atoi(a)
		if !ok {
			return nil, r.waitBadJob(a)
		}
		found := false
		waitable := r.waitableByIdent(pid)
		if r.unspecified {
			return nil, r.status
		}
		for _, j := range waitable {
			jobs = append(jobs, j)
			found = true
		}
		if !found {
			if w := r.diag().WaitNotOurChild; w != "" {
				r.diagf("%s\n", Wording(w, "", pid))
			}
			return nil, 127
		}
	}
	return jobs, 0
}

// storeWaitedPID is `-p`'s half: the process id of the job the status came
// from, written through the same store an assignment uses so that
// `wait -p A[$key] -n %2` reaches an associative element.
//
// A nil job empties the name rather than leaving it alone, which is measured:
// `V=preset; wait -p V` with nothing to wait for leaves V empty in bash
// 5.3.15. So the letter always writes once it was given.
func (r *Runner) storeWaitedPID(opts waitOpts, j *Job) {
	if !opts.named || opts.pvar == "" {
		return
	}
	value := ""
	if j != nil {
		value = strconv.Itoa(j.Ident())
	}
	r.storeThroughOperand(opts.pvar, value)
}

// reportStoppedWait says a wait gave up because the job stopped, in whichever
// of the two wordings the caller is. Nothing is said where the dialect has no
// wording, which is every dialect that does not give up at all.
func (r *Runner) reportStoppedWait(j *Job, wording string, extra ...any) {
	if wording == "" {
		return
	}
	args := append([]any{r.jobNumber(j) + 1}, extra...)
	r.diagf("%s\n", Wording(wording, "", args...))
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

// waitJobSpecNaming waits for the job a `%` spec names, and hands the job
// back so that `-p` can name it.
//
// The job is returned only where one was found and waited out: a spec that
// names nothing, or a wait that gave up, leaves `-p` with nothing to write
// but the empty string.
func (r *Runner) waitJobSpecNaming(spec string) (int, *Job) {
	j, code := r.findJobQuietly(spec)
	switch code {
	case jobFound:
		st, sig, hit, stopped := r.waitFor(j)
		if r.unspecified {
			return r.status, nil
		}
		if hit {
			// The wait did not finish, so the job is not finished with
			// either and stays in the table for the next one.
			return r.interruptedWaitStatus(sig, true), nil
		}
		if stopped {
			// Kept for the same reason, and said out loud on this route:
			// the wait gave up, the job is still there stopped, and `fg`
			// and `bg` can still name it.
			r.reportStoppedWait(j, r.diag().WaitForJobStopped)
			return r.stoppedWaitStatus(j), nil
		}
		r.reap(j)
		r.noticeWaitedSignal(j, st)
		return st, j
	case jobSpecAmbiguous:
		r.diagf("%s\n", Wording(r.diag().AmbiguousJobSpec,
			"%[1]s: %[2]s: ambiguous job spec", "wait", strings.TrimPrefix(spec, "%")))
		return orDefault(r.diag().WaitNoSuchJobStatus, 127), nil
	case jobSpecUnanswered:
		return r.status, nil
	}
	// A spec that names nothing: said and failed, or — in one shell —
	// nothing at all and 0.
	if !r.ask(r.sem().WaitReportsAMissingJob, "`wait` reporting a job spec that names nothing") {
		if r.unspecified {
			return r.status, nil
		}
		return 0, nil
	}
	r.diagf("%s\n", Wording(r.diag().WaitNoSuchJob, "wait: %[1]s: no such job", spec))
	return orDefault(r.diag().WaitNoSuchJobStatus, 127), nil
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
		// Already stopped and already noticed: this job was built *from* the
		// stop, so there is no note for the shell to take later. Nothing is
		// waiting on its process either — that is what `polled` says — so
		// nothing would ever close the channel.
		stopNote:    make(chan struct{}),
		noticedStop: true,
	}
	// Through settlePID rather than as a field, so that the field's one write
	// is the one the channel publishes. A job stopped in the foreground has
	// its process from the start; there is nothing left to settle later.
	//
	// With a group of its own, which is what a foreground command this shell
	// was watching always has: the group is how it came to be stopped at all
	// — see setProcessGroup's caller, where the foreground half does not ask
	// about the monitor.
	job.settleStartedPID(pid, true)
	r.addJob(job)
	r.setLastJob(job)
	r.becomeCurrentJob(job)
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
	r.toldOfJobsAtExit, r.tellingOfJobsAtExit = false, false
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
		r.errf("%s\n", Wording(w, "", i, r.jobMarker(j), r.name(), j.Command))
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
	// Before the poll and outside its guard, because it is a different
	// question: this is not asking the kernel anything, only taking what the
	// goroutine waiting on a `&` job's process has already been told. A
	// Runner with no PollCommand still has those goroutines.
	for _, j := range r.jobs {
		r.noticeStoppedJob(j)
	}
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

// noticeStoppedJob takes into the job what the goroutine waiting on its
// process saw, and reports whether the job is standing stopped now.
//
// The other half of Job.stopNote. The goroutine may only say that it happened;
// recording it is the shell's, on the shell's own thread of control, because
// Stopped and StopSig are read all over this package by a shell that is
// between statements and would otherwise be racing the job.
//
// Once, which is what keeps `bg` from being undone: a job the script resumed
// is running again, and a note that fired before it was resumed must not put
// it back to stopped the next time anything looks.
func (r *Runner) noticeStoppedJob(j *Job) bool {
	if j.noticedStop || j.Finished() {
		return j.Stopped && !j.Finished()
	}
	sig, saw := j.sawStop()
	if !saw {
		return false
	}
	j.noticedStop = true
	j.Stopped, j.StopSig = true, int(sig)
	return true
}

// HoldsExitForJobs reports whether this shell should stay rather than exit,
// because leaving now would abandon a job — and says so when it does.
//
// Exported because both ways out of a session reach it and only one of them is
// in this package: `exit` is a builtin, and the end of input is the front
// end's. Measured, the two are the same warning in the same words.
//
// Two kinds of job, asked in one place because the shells answer them in one
// sentence. A stopped job is the older half and is governed by
// Semantics.StoppedJobsHoldTheExit; a running one is
// Runner.ChecksRunningJobsAtExit, which both shells that hold spell
// `checkjobs`. Measured through a pseudo-terminal on 2026-09-08 against bash
// 5.3.15 and zsh 5.9.2, and every rule below is one of those runs:
//
//   - Stopped wins the sentence. With a job suspended *and* a `sleep 40 &` in
//     the table, bash says `There are stopped jobs.` and zsh says
//     `you have suspended jobs.` — neither mentions the running one, though
//     bash's listing under the sentence shows both.
//   - The running half needs the option and the stopped half does not, in
//     bash. `shopt -u checkjobs` still warns about a suspended job and no
//     longer warns about a running one.
//   - In zsh the option is the master of both, which is
//     Runner.ChecksStoppedJobsAtExit: `unsetopt checkjobs` and a suspended job
//     is an exit that happens.
//   - The status the held `exit` reports is the same for either kind — 1 in
//     bash, 0 in zsh — so Diagnostics.StoppedJobsAtExitStatus answers for
//     both and did not have to grow a twin.
//
// Once, and what "once" means is measured rather than assumed. The warning
// itself counts as having been told, so a second `exit` leaves; so does a
// `jobs` listing, which is the shell showing the same thing on purpose; and a
// job stopping afterwards starts the count again. Any other command in between
// does not — bash and zsh both warn again after an `echo`, and both warn again
// after the `echo $?` that reads the held exit's own status.
func (r *Runner) HoldsExitForJobs() bool {
	if !r.JobControl || r.toldOfJobsAtExit {
		return false
	}
	stopped, running := false, false
	for _, j := range r.jobs {
		switch {
		case j.Finished():
		case j.Stopped:
			stopped = true
		default:
			running = true
		}
	}
	// Stopped first, because that is the order the sentence is chosen in and
	// not merely the order the fields are declared in: a session with one of
	// each is told about the stopped one.
	wording := ""
	switch {
	case stopped && r.ChecksStoppedJobsAtExit():
		wording = Wording(r.diag().StoppedJobsAtExit, "there are stopped jobs", r.name())
	case running && r.ChecksRunningJobsAtExit():
		wording = Wording(r.diag().RunningJobsAtExit, "there are running jobs", r.name())
	default:
		return false
	}
	if !r.ask(r.sem().StoppedJobsHoldTheExit, "an exit held back by a job that would be abandoned") {
		// Unspecified is left to the caller: the axis has already complained,
		// and a shell that cannot say whether to stay had better leave than
		// refuse to.
		return false
	}
	// Both, and the second is what carries it to the next chunk: `exit` twice
	// leaves, and the end of input twice leaves without any chunk running
	// between the two.
	r.toldOfJobsAtExit, r.tellingOfJobsAtExit = true, true
	// Written plainly rather than through the dialect's location prefix: one
	// of the two shells that says this names itself in the sentence and the
	// other names nobody at all, and neither writes the line number a prompt's
	// diagnostics carry.
	r.errf("%s\n", wording)
	r.listJobsHeldAtExit()
	return true
}

// listJobsHeldAtExit prints the job table under the sentence, where the shell
// does that.
//
// Two conditions and they are different in kind. Whether this shell ever
// lists is Semantics.HeldExitListsTheJobs — bash does and zsh does not, in
// every case measured. Whether it lists *now* is the option, because bash's
// one `checkjobs` buys both halves of checking the jobs: with it off, the
// stopped-job sentence still appears and the table under it does not.
//
// Not asked through Runner.ask, because it is only ever reached in a shell
// that has already held an exit — the two dialects that never hold have no
// answer to give and are never put the question.
func (r *Runner) listJobsHeldAtExit() {
	if !r.ChecksRunningJobsAtExit() || r.sem().HeldExitListsTheJobs != Yes {
		return
	}
	// The same rows `jobs` writes, current-and-previous markers and all:
	// measured, the table under the sentence is `[1]+  Stopped ...` beside
	// `[2]-  Running ...`, which is the listing's own line and not a second
	// rendering of it.
	for _, j := range r.jobs {
		if j.Finished() {
			continue
		}
		r.errf("%s\n", r.jobLineAs(j.num, j, true, false))
	}
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

// setLastJob records a job's pid as `$!`.
//
// Only the pid. Which job the markers point at is jobOrder's, and the two are
// read apart because they stop being the same thing the moment the job ends —
// `$!` is never dropped, because no shell in the panel empties it, and a job
// that has left the table must not still answer `%%`. A disowned job is one
// of these and not the other: it is the most recent background job for `$!`
// and is in no table for `%%` to find.
func (r *Runner) setLastJob(j *Job) {
	r.lastJobPID, r.lastJobPIDSet = j.Ident(), true
}

// addJob puts a job in the table under a number of its own.
//
// The number is the job's identity and not its place in the table, which is
// the whole of what this is for. It used to be the place: `%2` meant the
// second element of the slice and a listing printed the index it was walking,
// so forgetting a job that had ended renumbered every job after it — and
// measured against bash 5.3.15, nothing renumbers. Three background jobs, the
// first reported as done and dropped, and bash still calls the third `%3`
// while this called it `%2`; a script that then said `kill %3` was told there
// was no such job and the process it meant ran to the end of its sleep. That
// is #2295 in the bash suite's own `jobs` file, several times over.
//
// Which number that is splits the panel, and it splits it only where the
// table has a *hole* in it — see nextJobNumber.
func (r *Runner) addJob(job *Job) {
	job.num = r.nextJobNumber()
	r.jobs = append(r.jobs, job)
}

// nextJobNumber is the number the next job entering the table takes: one past
// the highest occupied slot, or the lowest slot nobody holds.
//
// The two rules give the same answer for every table with no hole in it,
// which is nearly every table there has ever been — jobs are numbered from
// one and slots are freed from the top far more often than from the middle —
// and once the table empties both begin again at 1, the highest of nothing
// being zero. So the axis is asked **at the disagreement and nowhere else**:
// a shell whose jobs are 1 and 2 is not made to answer a question about
// holes, and a preset that has not chosen can run every script that never
// makes one.
//
// Where they differ, the answer is Semantics.NextJobNumberRefillsAHole and
// bash is alone in saying no. Measured 2026-09-12 on a script, three jobs
// started and the middle one killed and reaped:
//
//	sleep 5 & sleep 5 & sleep 5 &
//	kill %2; wait %2
//	sleep 5 &
//
// bash 5.3.15 and 3.2.57 leave `%2` empty and put the new job at `%4`; dash,
// ksh93u+, zsh 5.9.2 and BusyBox ash all put it back in `%2` and have no
// `%4`. The reading is discriminating rather than inferred from a listing:
// `jobs %2` is asked *before* the new job as well, so a shell that never
// freed the slot is not counted as one that refilled it.
func (r *Runner) nextJobNumber() int {
	high, taken := 0, make(map[int]bool, len(r.jobs))
	for _, j := range r.jobs {
		taken[j.num] = true
		if j.num > high {
			high = j.num
		}
	}
	lowest := 1
	for taken[lowest] {
		lowest++
	}
	if lowest == high+1 {
		// No hole, so the two rules agree and there is nothing to ask.
		return lowest
	}
	if r.ask(r.sem().NextJobNumberRefillsAHole, "the number a job takes when the table has a hole in it") {
		return lowest
	}
	return high + 1
}

// becomeCurrentJob puts a job on top of the order the markers are read from:
// it has just entered the table, or it has just stopped.
//
// Moved rather than appended where it is already there, so that a job which
// stops a second time is newer than one that stopped after it started.
// Measured through a pseudo-terminal, 2026-09-12: two jobs stopped in turn
// and then the *first* brought forward and stopped again reads `[1]+ [2]-` in
// every shell in the panel, where the order they were started in would give
// the opposite.
//
// Jobs the table no longer holds are dropped on the way past, which is the
// only cleanup this list needs: a marker names a job a script can still
// reach, and nothing else keeps these pointers alive.
func (r *Runner) becomeCurrentJob(j *Job) {
	kept := r.jobOrder[:0]
	for _, other := range r.jobOrder {
		if other != j && slices.Contains(r.jobs, other) {
			kept = append(kept, other)
		}
	}
	for i := len(kept); i < len(r.jobOrder); i++ {
		r.jobOrder[i] = nil
	}
	r.jobOrder = append(kept, j)
}

// markedJobs are the two jobs a listing marks: `+` on the one `fg` would pick
// with no operand — `%%` and `%+` — and `-` on the one that would take its
// place — `%-`.
//
// Both come off jobOrder rather than off the table, and both go through the
// same choice, which is what makes `-` "the runner-up" rather than "the one
// before it in the table".
//
// The choice itself is the axis. Measured 2026-09-12 through a pseudo-terminal
// with a scratch home directory, on a job stopped with ^Z and then a `sleep &`
// started after it:
//
//	bash 5.3.15  [1]+ Stopped   [2]-  Running
//	bash 3.2.57  [1]+ Stopped   [2]-  Running
//	dash         [1]+ Suspended [2]-  Running
//	zsh 5.9.2    [1]+ suspended [2]-  running
//	ksh93u+      [1]- Stopped   [2]+  Running
//
// So in five of the six columns a stopped job keeps the marker and a later
// background job does not take it; in one, the marker is simply on the newest
// job. `%+` and `%-` resolve to match in each — asked directly, they name the
// same two jobs the listing marks — so this is not a cosmetic column: a
// `fg %+` after a ^Z resumes a different job in the two camps.
func (r *Runner) markedJobs() (current, previous *Job) {
	current = r.pickMarkedJob(nil)
	if current != nil {
		previous = r.pickMarkedJob(current)
	}
	return current, previous
}

// pickMarkedJob is one step of that choice, skipping a job already marked.
//
// Read rather than `ask`ed, the way the letters of `$-` are: naming the
// default job is not the place to refuse a script over a disagreement, and a
// dialect that answers nothing gets the answer five of the six columns give.
func (r *Runner) pickMarkedJob(skip *Job) *Job {
	stopped := r.sem().StoppedJobTakesTheCurrentJobMarker != No
	var newest *Job
	for i := len(r.jobOrder) - 1; i >= 0; i-- {
		j := r.jobOrder[i]
		if j == skip || !slices.Contains(r.jobs, j) {
			continue
		}
		if stopped && j.Stopped {
			return j
		}
		if newest == nil {
			newest = j
		}
	}
	return newest
}

// currentJob is the job `%%`, `%+` and a bare `fg` name, or nil where this
// shell has none.
func (r *Runner) currentJob() *Job { current, _ := r.markedJobs(); return current }

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

// reap is Forget for a job a `wait` has just reported the status of: the job
// leaves the table, and is kept where a later `wait` naming the same id can
// still find it.
//
// **The leaving is the point, and it is unanimous.** `%1` is a slot in the
// table the shell holds *now*, not the first job it ever started, and a job
// that has been waited out is not in it. Measured 2026-09-13 across the seven
// columns — bash 5.3.15, that bash invoked as `sh`, bash 3.2.57, zsh 5.9.2,
// ksh93u+ 2012-08-01, dash and BusyBox ash 1.37.0 — on
//
//	/bin/sh -c 'exit 7' & p=$!
//	wait "$p"
//	/bin/sh -c 'exit 4' &
//	wait %1
//
// every one of them answers 4, and every one of them answers `wait %2` with
// its no-such-job status: the new job took the number the reaped one had. This
// shell answered 7 and numbered the new job `%2`, because waiting by process
// id left the finished job sitting in the table forever (#2651). Waiting by
// `%` spec already dropped it, which is why the same script written `wait %1`
// throughout was right and the ordinary one was wrong.
//
// **The keeping is a second question with a second answer.** A job reaped by
// name is still waitable by id in six of those seven columns — measured on
// `/bin/sh -c 'exit 7' & p=$!; wait %1; wait "$p"`, which answers 7 everywhere
// but ksh93u+, where it is 127. See Semantics.WaitRemembersAReapedJob, and the
// narrower reading two columns hold that is not modeled here.
func (r *Runner) reap(j *Job) {
	r.Forget(j)
	if slices.Contains(r.reaped, j) {
		// Already remembered: a second `wait` for the same id reaches this
		// through the memory itself, and a job listed twice would only push
		// an older one out sooner.
		return
	}
	// Ahead of the older ones, so that the bound below drops the oldest.
	r.reaped = append(r.reaped, j)
	if extra := len(r.reaped) - reapedJobsKept; extra > 0 {
		// Cleared as they go, so that a job nobody can name again is not held
		// alive by the slice that has already let go of it.
		for i := range r.reaped[:extra] {
			r.reaped[i] = nil
		}
		r.reaped = append(r.reaped[:0], r.reaped[extra:]...)
	}
}

// reapedJobsKept bounds that memory.
//
// bash does not appear to bound it at all: measured 2026-09-13, `wait "$first"`
// still answers 7 after five thousand further jobs have been started and
// reaped. A bound is taken here anyway, because a session that starts a job a
// second would otherwise grow a list for as long as it runs, and a script that
// waits on an id it reaped a thousand jobs ago is past anything the panel was
// measured doing. dash and BusyBox ash need none of it: their memory is the
// table entry itself, which the next job's number evicts.
const reapedJobsKept = 1024

// waitableByIdent is the jobs a `wait` naming a process id may report: the
// ones the table holds under that number, and the reaped one where the dialect
// still remembers it.
//
// Several rather than one, because a number can name more than one job here
// and used to name many: every job with no process of its own answered to 0,
// so `wait 0` reached all of them at once and reported whichever came last.
// Job.Ident is what closed that, and the shape stays because a caller must not
// have to know it did.
func (r *Runner) waitableByIdent(pid int) []*Job {
	var jobs []*Job
	for _, j := range r.jobs {
		if j.Ident() == pid {
			jobs = append(jobs, j)
		}
	}
	if len(jobs) > 0 {
		return jobs
	}
	// A process substitution's body, where `$!` named one. Only by number,
	// which is the only way a script can reach it. See
	// Semantics.ProcessSubstitutionIsTheLastBackgroundJob.
	for _, j := range r.procSubJobs {
		if j.Ident() == pid {
			jobs = append(jobs, j)
		}
	}
	if len(jobs) > 0 {
		return jobs
	}
	// The memory, and the axis asked *at the disagreement and nowhere else*:
	// only a number this shell has actually reaped a job under is a number
	// the columns answer differently, so a `wait` for a process that was
	// never this shell's is the same complaint it always was rather than a
	// refusal over a question the script never reached.
	var kept []*Job
	for _, j := range r.reaped {
		if j.Ident() == pid {
			kept = append(kept, j)
		}
	}
	if len(kept) == 0 {
		return nil
	}
	if !r.ask(r.sem().WaitRemembersAReapedJob, "`wait` on the id of a job it has already reported") {
		return nil
	}
	return kept
}

// jobByIdent is the job a plain number names, where that number is one this
// shell invented for a job with no process of its own.
//
// Only an invented one. A real process id is not looked up here at all: it may
// be this shell's child and it may be anything else on the machine, and a
// `kill` aimed at it is aimed at the process whatever this shell thinks of it.
// The invented numbers are out above every id a kernel can issue precisely so
// that this test is a test and not a guess — see jobident.go.
//
// The table only. A job that has been reaped has no processes left to signal,
// so finding it would change nothing about what `kill` does.
func (r *Runner) jobByIdent(n int) *Job {
	if n < inventedJobIdentBase {
		return nil
	}
	for _, j := range r.jobs {
		if j.Ident() == n {
			return j
		}
	}
	return nil
}

// Forget drops a job the shell has finished with — one that has been resumed
// into the foreground and ended, or reported as done.
func (r *Runner) Forget(j *Job) {
	for i, other := range r.jobs {
		if other == j {
			r.jobs = append(r.jobs[:i], r.jobs[i+1:]...)
			break
		}
	}
	for i, other := range r.jobOrder {
		if other == j {
			r.jobOrder = append(r.jobOrder[:i], r.jobOrder[i+1:]...)
			break
		}
	}
}

// jobProcess is one of the processes a job is made of: what to signal, and
// whether the number names a process group or a single process.
type jobProcess struct {
	pid      int
	ownGroup bool
}

// took records a process this job is now made of.
//
// A job is not one process. A backgrounded pipeline is several, and `%1` names
// all of them — measured against bash 5.3.15, `sleep 6 | cat & kill %1` ends
// both halves whether the monitor is on or off. Only [Job.PID] is one number,
// because `$!` is one number; what `kill %1` has to reach is this list.
//
// Appended in start order and pruned as each process is waited for, so a
// background loop that starts a thousand commands holds one entry rather than
// a thousand. The pruning is why the readers below fall back to [Job.PID]: a
// job between two commands is made of nothing at that instant, and a `kill`
// that arrived then used to reach the pid the job settled with, so it still
// does.
func (j *Job) took(pid int, ownGroup bool) {
	j.procsMu.Lock()
	defer j.procsMu.Unlock()
	j.procs = append(j.procs, jobProcess{pid: pid, ownGroup: ownGroup})
}

// released drops a process this job was made of, once it has been waited for.
func (j *Job) released(pid int) {
	j.procsMu.Lock()
	defer j.procsMu.Unlock()
	j.procs = slices.DeleteFunc(j.procs, func(p jobProcess) bool { return p.pid == pid })
}

// processes is what signaling this job has to reach, newest last.
//
// The settled pid where the list is empty, which is the pre-pipeline answer
// and stays the answer for a job that has no live process of its own just now.
// Empty means there is nothing to signal at all, and the callers word that as
// the job they cannot find.
func (j *Job) processes() []jobProcess {
	j.procsMu.Lock()
	live := slices.Clone(j.procs)
	j.procsMu.Unlock()
	if len(live) > 0 {
		return live
	}
	// Outside the lock, because this is a wait and the goroutine it is
	// waiting for takes that lock on its way past.
	<-j.ready
	if j.PID == 0 {
		return nil
	}
	return []jobProcess{{pid: j.PID, ownGroup: j.ownGroup}}
}

// tookJobProcess records a process this shell has just started against the
// background job it is part of, and does nothing where it is part of none.
//
// The job rather than the runner, because the two are not the same set: the
// runner that *names* a backgrounded pipeline is its last element, and the
// `sleep` in `sleep 6 | cat &` is a process of the job all the same. It was
// not recorded anywhere before, so `kill %1` reached the `cat` and left the
// `sleep` to run out its six seconds — which is the whole of #2295, multiplied
// by every such pipeline in a suite file.
func (r *Runner) tookJobProcess(pid int, ownGroup bool) {
	if r.inJob != nil {
		r.inJob.took(pid, ownGroup)
	}
	// A piece of the job with a process of its own has started, whatever else
	// it goes on to do.
	r.part.started()
}

// releasedJobProcess is tookJobProcess undone, once the process has been
// waited for. See Job.took for why the list is pruned rather than kept.
func (r *Runner) releasedJobProcess(pid int) {
	if r.inJob != nil {
		r.inJob.released(pid)
	}
}

// startAndWait is exec.Cmd.Run with the pid recorded against the job in
// between, which is the one thing Run leaves no room for.
func (r *Runner) startAndWait(cmd *exec.Cmd, ownGroup bool) error {
	if err := r.startMasked(cmd); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	r.tookJobProcess(pid, ownGroup)
	defer r.releasedJobProcess(pid)
	return cmd.Wait()
}
