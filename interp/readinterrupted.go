// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"context"
	"io"
	"os"
	"syscall"
	"time"

	"github.com/blairham/sh/internal/fdset"
)

// ReadAbandonmentReading is *when* a trapped signal gives up a `read`, in the
// dialects that give one up at all. See Semantics.ReadAbandonedWhen.
//
// Measured 2026-10-29 against dash 0.5.12, BusyBox ash 1.37.0 in the pinned
// alpine image and ksh93u+ 2012-08-01; docs/spec/semantics.md has the tables.
type ReadAbandonmentReading int

const (
	// ReadAbandonmentUnspecified is no answer, and is refused like any other —
	// once a trapped signal has arrived during a read, which is the only place
	// the readings part.
	ReadAbandonmentUnspecified ReadAbandonmentReading = iota
	// ReadAbandonedAtTheNextByte gives the read up at the next byte it asks
	// for once a trapped signal has arrived, whether that byte is there or
	// not and however much of the line is in. It is the reading this shell
	// had before #7027, and the one ReadAbandonedOnlyAtTheStartOfALine falls
	// back to on the reads it does not cover.
	ReadAbandonedAtTheNextByte
	// ReadAbandonedOnlyDuringAWait gives the read up only for a signal that
	// arrives while it is waiting for input. One that arrives while input is
	// there to be read does not, and nor does it give up a wait that begins
	// after it: the read goes on, and the handler runs once it is over. A
	// second signal that arrives during that later wait does give it up.
	// dash and BusyBox ash.
	//
	// So whether a line still arriving is finished is a matter of whether
	// the reader happened to be waiting at the instant the signal landed. A
	// writer that keeps the stream full makes it deterministic; one that
	// lets it run dry now and then does not, and dash on a pipe fed by `tr`
	// gave a 20 MB line up partway in three runs of four.
	ReadAbandonedOnlyDuringAWait
	// ReadAbandonedOnlyAtTheStartOfALine gives a read of the builtin's
	// standard input, from a descriptor open for reading alone, that reads
	// to a delimiter, up only for a signal that lands while it waits for the
	// first byte of a line — at its start, or just after a newline it has
	// read, which is where a continued line or a `-d` record with a newline
	// in it starts another. Once a byte of the line is in, the read goes on
	// to the line's end, the deadline of a `-t` still bounding it, and no
	// signal gives it up. ksh93.
	//
	// A read given its stream by `-u` or `-p`, or one that counts with `-n`
	// or `-N`, is given up at the next byte by a signal that arrives during
	// it. Neither kind is given up by a signal already waiting when it
	// began: the read goes on. A read from a descriptor open for writing as
	// well — `exec 3<>fifo` — is given up as ReadAbandonedAtTheNextByte
	// gives one up, a signal already waiting included.
	ReadAbandonedOnlyAtTheStartOfALine
)

func (a ReadAbandonmentReading) String() string {
	switch a {
	case ReadAbandonedAtTheNextByte:
		return "ReadAbandonedAtTheNextByte"
	case ReadAbandonedOnlyDuringAWait:
		return "ReadAbandonedOnlyDuringAWait"
	case ReadAbandonedOnlyAtTheStartOfALine:
		return "ReadAbandonedOnlyAtTheStartOfALine"
	}
	return "ReadAbandonmentUnspecified"
}

// readWatch is one `read`'s watch for the trapped signal that gives it up, in
// the dialect that gives one up, and for the handlers a trapped signal runs in
// the middle of a read that goes on. See
// Semantics.ReadIsAbandonedByATrappedSignal, Semantics.ReadAbandonedWhen and
// Semantics.ReadHandlersRun.
//
// **The dialect is asked only once a signal has arrived.** Every reading
// waits for a byte, and reads it, exactly alike until then, so a read nothing
// arrives during — every read of an ordinary script — asks nothing at all.
type readWatch struct {
	r  *Runner
	in io.Reader
	// byNumber is a read handed its stream by `-u` or `-p`, and counted one
	// that reads to a count rather than to a delimiter. Only
	// ReadAbandonedOnlyAtTheStartOfALine looks at either.
	byNumber, counted bool
	// reading is the dialect's answer, and asked says it has been asked.
	reading ReadAbandonmentReading
	asked   bool
	// inLine is true once a byte of the current line has been read, and
	// false again after a newline.
	inLine bool
	// began is arrivalMark as the read began: the arrivals before it are not
	// the read's, in the readings that tell the two apart.
	began int
	// bothWays is whether the stream is a descriptor open for writing as well
	// as reading, worked out the first time it matters: 0 not yet, 1 yes, -1
	// no. Only ReadAbandonedOnlyAtTheStartOfALine asks.
	bothWays int8
	// goesOn is the watch of a read in a dialect whose reads go on through a
	// trapped signal: nothing it sees gives the read up, and it is kept for
	// the handlers the dialect runs in the middle of a read
	// (Semantics.ReadHandlersRun). timed is a `read -t`, whose wait for its
	// first byte the dialect may end as the deadline would
	// (Semantics.ReadTimeoutWaitEndedByATrappedSignal); ends is that answer,
	// and asked says it has been asked.
	goesOn, timed, ends bool
	// started is true once a byte has been read: the wait a signal may end
	// in a timed read that goes on is the one for the first byte.
	started bool
	// moment is the dialect's answer to when a handler runs during a read
	// that goes on, asked the first time a trapped signal is waiting there;
	// momentAsked says it has been.
	moment      ReadHandlerMoment
	momentAsked bool
	// ended is why the current wait ended on an arrival, set by the
	// question that wait asks. See readVerdict.
	ended readVerdict
	// ctx is the builtin's, for the handlers run in the middle of the read.
	ctx context.Context
	// deathOnly is the watch of a read nothing trapped can give up, kept for
	// a fatal signal from outside that the shell has been handed to die of:
	// every wait for a byte is given up for that, and for nothing else. See
	// outsidedeath.go.
	deathOnly bool
}

// readVerdict is why a wait of a watched read ended on an arrival.
type readVerdict uint8

const (
	// readGivenUp gives the read up, as at the end of its input — for a
	// trapped signal in the dialects whose reads are given up, and for a
	// fatal signal from outside in any. See readWatch.gaveUp.
	readGivenUp readVerdict = iota
	// readTimedOut ends a timed read's wait for its first byte as its
	// deadline would. See Semantics.ReadTimeoutWaitEndedByATrappedSignal.
	readTimedOut
	// readRunsHandlers stops the read to run the handlers waiting, and then
	// goes on. See Semantics.ReadHandlersRun.
	readRunsHandlers
	// readRefused ends the read over an axis with no answer, which has
	// already been reported.
	readRefused
)

// readWatchFor is the watch for a read of in, and false where nothing can stop
// that read: no signal is trapped, and no fatal signal from outside could be
// handed over either — or the stream is a regular file, whose reads do not
// wait for anybody.
func (r *Runner) readWatchFor(ctx context.Context, in io.Reader, byNumber, counted bool) (*readWatch, bool) {
	if w, ok := r.trapWatchFor(ctx, in, byNumber, counted); ok {
		return w, true
	}
	if w, ok := r.goesOnWatchFor(ctx, in); ok {
		return w, true
	}
	return r.deathWatchFor(in)
}

// trapWatchFor is readWatchFor for the trapped signals, in the dialect whose
// reads they give up.
func (r *Runner) trapWatchFor(ctx context.Context, in io.Reader, byNumber, counted bool) (*readWatch, bool) {
	if r.sem().ReadIsAbandonedByATrappedSignal != Yes || !r.anySignalTrapped() || neverWaits(in) {
		return nil, false
	}
	return &readWatch{r: r, in: in, byNumber: byNumber, counted: counted, began: r.arrivalMark(), ctx: ctx}, true
}

// goesOnWatchFor is readWatchFor for the trapped signals, in the dialect whose
// reads go on through them: none gives the read up, and the watch is there
// for the handlers that may run in the middle of it. See
// Semantics.ReadHandlersRun.
//
// It is made wherever a signal is trapped and asks nothing until one is
// waiting during the read — so a read nothing arrives during, which is every
// read of a script that is not being signaled, puts no question at all.
func (r *Runner) goesOnWatchFor(ctx context.Context, in io.Reader) (*readWatch, bool) {
	if r.sem().ReadIsAbandonedByATrappedSignal == Yes || !r.anySignalTrapped() || neverWaits(in) {
		return nil, false
	}
	return &readWatch{r: r, in: in, goesOn: true, ctx: ctx}, true
}

// deathWatchFor is the watch of a read that only a fatal signal from outside
// can give up, where the shell has an EXIT trap one would be handed over for.
// A read in a shell with no such trap is not watched for it, and keeps the
// plainer source. See outsidedeath.go.
func (r *Runner) deathWatchFor(in io.Reader) (*readWatch, bool) {
	if r.abandonsForADeath() == nil || neverWaits(in) {
		return nil, false
	}
	return &readWatch{r: r, in: in, deathOnly: true}, true
}

// neverWaits reports whether in is a regular file, which never makes a
// reader wait.
func neverWaits(in io.Reader) bool {
	if f, isFile := in.(*os.File); isFile {
		if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// timedReadWatchFor is the watch for a `read -t` of in, nil where nothing can
// stop that read early: readWatchFor's, with the wait for the first byte of a
// read that goes on also one a trapped signal may end. See
// Semantics.ReadTimeoutWaitEndedByATrappedSignal.
func (r *Runner) timedReadWatchFor(ctx context.Context, in io.Reader, byNumber, counted bool) *readWatch {
	w, ok := r.readWatchFor(ctx, in, byNumber, counted)
	if !ok {
		return nil
	}
	w.timed = w.goesOn
	return w
}

// readingNow is the dialect's answer, asked the first time it is needed. An
// unanswered axis is refused there: the read ends, and the builtin reports 2.
func (w *readWatch) readingNow() ReadAbandonmentReading {
	if !w.asked {
		w.asked = true
		w.reading = w.r.sem().ReadAbandonedWhen
		if w.reading == ReadAbandonmentUnspecified {
			w.r.diagf("%s\n", w.r.unanswered("when a trapped signal gives up a `read`"))
			w.r.status = 2
			w.r.unspecified = true
		}
	}
	return w.reading
}

// openBothWays reports that the stream is a descriptor open for writing as
// well as reading, asked of the kernel once.
func (w *readWatch) openBothWays() bool {
	if w.bothWays == 0 {
		w.bothWays = -1
		if f, ok := w.in.(interface{ Fd() uintptr }); ok && descriptorOpenForWriting(int(f.Fd())) {
			w.bothWays = 1
		}
	}
	return w.bothWays > 0
}

// startOfALine is ReadAbandonedOnlyAtTheStartOfALine's answer for a signal
// that has arrived, mark being where the arrivals that count start: the
// current wait's for a line read, and nil where no byte is being waited for.
// handled is false where the reading falls back to the next byte.
func (w *readWatch) startOfALine(mark *int) (sig syscall.Signal, gives, handled bool) {
	if w.openBothWays() {
		return 0, false, false
	}
	if w.byNumber || w.counted {
		sig, gives = w.r.pendingTrapFrom(w.began)
		return sig, gives, true
	}
	if mark == nil || w.inLine {
		return 0, false, true
	}
	sig, gives = w.r.pendingTrapFrom(*mark)
	return sig, gives, true
}

// trapWaiting is a trapped signal that has arrived and not yet been handled,
// looked for without the lock where nothing has. A subshell's own waits are
// never cut short — see awaitOrTrap — and neither are its reads.
func (w *readWatch) trapWaiting() (syscall.Signal, bool) {
	s := w.r.signals
	if s == nil || w.r.inSubshell || s.nothingArrived() {
		return 0, false
	}
	return w.r.pendingTrap()
}

// givesUpWithAByteThere reports the signal that gives the read up at a byte
// that is already there to be read. Only a reading that does not wait for a
// wait to be interrupted does.
func (w *readWatch) givesUpWithAByteThere() (syscall.Signal, bool) {
	if w.goesOn || w.deathOnly {
		// A byte there is a wait that never began, and a death from
		// outside is taken at the next command either way.
		return 0, false
	}
	sig, ok := w.trapWaiting()
	if !ok {
		return 0, false
	}
	switch w.readingNow() {
	case ReadAbandonedOnlyDuringAWait:
		return 0, false
	case ReadAbandonedOnlyAtTheStartOfALine:
		if sig, gives, handled := w.startOfALine(nil); handled {
			return sig, gives
		}
	}
	return sig, true
}

// handlersRunWithAByteThere is what a read that goes on does at a byte that is
// already there to be read, with a trapped signal waiting: run the handlers
// first, in the dialect that runs them as soon as they arrive, and otherwise
// read on. readRefused is an unanswered axis, already reported.
func (w *readWatch) handlersRunWithAByteThere() (readVerdict, bool) {
	if w.deathOnly {
		return 0, false
	}
	if _, ok := w.trapWaiting(); !ok {
		return 0, false
	}
	switch w.momentNow() {
	case ReadHandlersRunAsSoonAsTheyArrive:
		return readRunsHandlers, true
	case ReadHandlerMomentUnspecified:
		return readRefused, true
	}
	return 0, false
}

// arrivedDuring is the question a wait for the next byte asks each time
// something may have arrived, mark being arrivalMark as the wait began. It
// leaves why the wait ended in w.ended.
//
// A signal that came before the wait does not end it in the reading that
// gives a read up only during one — it is still on the pending list, and it
// runs once the read is over — so that reading looks only past the mark.
func (w *readWatch) arrivedDuring(mark int) func() (syscall.Signal, bool) {
	if w.deathOnly {
		// No trapped arrival stops this read; the wait itself is given up
		// for a death from outside. See Runner.awaitOrArrival.
		return func() (syscall.Signal, bool) { return 0, false }
	}
	if w.goesOn {
		return func() (syscall.Signal, bool) {
			if w.timed && !w.started {
				if sig, ends := w.endsTheFirstWait(mark); ends {
					w.ended = readTimedOut
					return sig, true
				}
			}
			return w.handlersRunDuringAWait(mark)
		}
	}
	return func() (syscall.Signal, bool) {
		sig, ok := w.r.pendingTrap()
		if !ok {
			return 0, false
		}
		gives := true
		switch w.readingNow() {
		case ReadAbandonedOnlyDuringAWait:
			sig, gives = w.r.pendingTrapFrom(mark)
		case ReadAbandonedOnlyAtTheStartOfALine:
			if at, g, handled := w.startOfALine(&mark); handled {
				sig, gives = at, g
			}
		}
		if gives || w.r.unspecified {
			w.ended = readGivenUp
			return sig, true
		}
		return w.handlersRunDuringAWait(mark)
	}
}

// handlersRunDuringAWait is the arrival that stops a wait of a read that goes
// on, to run the handlers waiting: any of them, in the dialect that runs them
// as soon as they arrive, and one that landed in this wait in the dialect that
// runs them when a wait is interrupted. The dialect is asked only once a
// trapped signal is waiting; an unanswered axis ends the read.
func (w *readWatch) handlersRunDuringAWait(mark int) (syscall.Signal, bool) {
	sig, ok := w.r.pendingTrap()
	if !ok {
		return 0, false
	}
	switch w.momentNow() {
	case ReadHandlersRunAsSoonAsTheyArrive:
	case ReadHandlersRunWhenAWaitIsInterrupted:
		if sig, ok = w.r.pendingTrapFrom(mark); !ok {
			return 0, false
		}
	case ReadHandlerMomentUnspecified:
		w.ended = readRefused
		return sig, true
	default:
		return 0, false
	}
	w.ended = readRunsHandlers
	return sig, true
}

// momentNow is the dialect's answer to when a handler runs during a read that
// goes on, asked the first time it is needed. An unanswered axis is refused
// there, and the read ends so that the builtin can report the refusal.
func (w *readWatch) momentNow() ReadHandlerMoment {
	if !w.momentAsked {
		w.momentAsked = true
		w.moment = w.r.sem().ReadHandlersRun
		if w.moment == ReadHandlerMomentUnspecified {
			w.r.diagf("%s\n", w.r.unanswered("when the handler of a signal that arrives during a `read` runs"))
			w.r.status = 2
			w.r.unspecified = true
		}
	}
	return w.moment
}

// runHandlers runs the handlers waiting, in the middle of the read, and
// reports whether the read ends there: a handler left the shell — `exit`, a
// `return` out of the function the read is in — or one was refused. A read
// that ends so touches no name; the builtin hands back the status the
// handler left. See Semantics.ReadHandlersRun.
//
// **The handler sees `$?` as it stands.** The read has not finished, so it
// has no status of its own for the dialect that shows a handler the status
// from before the command (Semantics.SignalHandlerSeesEarlierStatus) to look
// past: measured 2026-10-31, zsh's handler mid-read reads 1 after `false`, as
// bash's does, and 0 after a command substitution in the read's own
// redirection, as bash's does.
//
// Everything waiting is taken, so what arrives from here on is the read's: the
// mark of the arrivals before it began is moved to the front.
func (w *readWatch) runHandlers() bool {
	r := w.r
	before := r.statusBefore
	r.statusBefore = r.status
	r.runPendingTraps(w.ctx)
	r.statusBefore = before
	w.began = 0
	return r.ctl != controlNone || r.unspecified
}

// endsTheFirstWait is the question of a timed read that goes on, asked of a
// wait for its first byte: whether the trapped signal that landed in it ends
// the read as the deadline would. Only one that lands during that wait is
// looked at, and the dialect is asked about it then. A signal handled before
// the read began is not one — measured, zsh reads on over it — and nor is one
// already waiting, which pendingTrapFrom leaves out.
//
// An unanswered axis is refused there, and the read ends so that the builtin
// can report the refusal.
func (w *readWatch) endsTheFirstWait(mark int) (syscall.Signal, bool) {
	sig, ok := w.r.pendingTrapFrom(mark)
	if !ok {
		return 0, false
	}
	if !w.asked {
		w.asked = true
		w.ends = w.r.ask(w.r.sem().ReadTimeoutWaitEndedByATrappedSignal,
			"a trapped signal ending a `read -t` still waiting for its first byte")
	}
	return sig, w.ends || w.r.unspecified
}

// gaveUp ends the read for the arrival that ended its wait, sig being the
// signal, as w.ended says.
//
// A read given up ends as at the end of the input: nothing more is read, and
// the handler runs once the builtin has returned, like any other trap between
// commands. A timed read's first wait ended by a signal ends as the deadline
// would instead, which is what that reading is — the status is the timeout's
// and no name is touched — and leaves the read's status nothing of the
// signal's. A refused axis ends it as at the end of input too, with the
// refusal already said.
func (w *readWatch) gaveUp(sig syscall.Signal) int {
	switch w.ended {
	case readTimedOut:
		return evTimeout
	case readRefused:
		return evEOF
	}
	w.r.readAbandonedBy = sig
	return evEOF
}

// track follows the bytes a source hands over, for the line the reading that
// keeps one needs to know is under way, and holds the source at its end once
// it has ended.
func (w *readWatch) track(read func() (byte, int)) func() (byte, int) {
	ended, last := false, 0
	return func() (byte, int) {
		if ended {
			return 0, last
		}
		b, ev := read()
		if ev != evByte {
			ended, last = true, ev
			return b, ev
		}
		w.inLine = b != '\n'
		w.started = true
		return b, ev
	}
}

// awaitByte is one wait of a read on fd the watch is on: until fd has a byte
// for it, the deadline passes, or a trapped signal gives the read up. A zero
// deadline is none, and then only a byte or a signal ends the wait — whether
// fd could be asked at all is for the read that follows to find out.
//
// **A read given up is not left parked on the stream.** The waiting for a
// byte happens on a goroutine and the waiting for a signal in awaitOrArrival,
// which is what makes `wait` interruptible and already knows every way a
// signal reaches this shell. The goroutine only asks whether a byte is there
// and never reads one, so letting it go is all a give-up has to do: it notices
// within canceledReadNoticedWithin and returns having taken nothing (#7024).
//
// Where the byte is already there no goroutine is started, and the readings
// part over a signal already waiting: see givesUpWithAByteThere.
//
// **A read that stops to run handlers waits again afterwards**, for whatever
// is left of the same deadline: the read goes on as though the handlers had
// taken no time out of it, and a handler that left the shell ends it. See
// Semantics.ReadHandlersRun.
func (w *readWatch) awaitByte(ctx context.Context, fd int, deadline time.Time) int {
	for {
		if ready, _ := fdset.ReadableWithin(fd, 0); ready {
			if sig, gives := w.givesUpWithAByteThere(); gives {
				return w.gaveUp(sig)
			}
			verdict, stops := w.handlersRunWithAByteThere()
			switch {
			case !stops:
				return evByte
			case verdict == readRefused || w.runHandlers():
				return evEOF
			}
			// The handler may have read the stream itself, so the byte is
			// looked for again.
			continue
		}
		sig, trapped, ready, can := w.waitOnce(ctx, fd, deadline)
		if trapped {
			if w.ended != readRunsHandlers {
				return w.gaveUp(sig)
			}
			if w.runHandlers() {
				return evEOF
			}
			continue
		}
		switch {
		case deadline.IsZero():
			return evByte
		case !can:
			return evEOF
		case !ready:
			return evTimeout
		}
		return evByte
	}
}

// waitOnce is one wait of awaitByte's: until fd is readable, the deadline
// passes, or an arrival ends it, which trapped reports and w.ended explains.
//
// The goroutine's answer is read only once it has closed readable; a wait
// ended by an arrival leaves it to notice the cancel and go, unread.
func (w *readWatch) waitOnce(
	ctx context.Context,
	fd int,
	deadline time.Time,
) (sig syscall.Signal, trapped, ready, can bool) {
	mark := w.r.arrivalMark()
	readable := make(chan struct{})
	var isReady, canAsk bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer close(readable)
		isReady, canAsk = awaitReadable(ctx, fd, deadline)
	}()
	w.ended = readGivenUp
	if sig, trapped, _ := w.r.awaitOrArrival(readable, nil, w.arrivedDuring(mark)); trapped {
		return sig, true, false, false
	}
	return 0, false, isReady, canAsk
}

// trapInterruptibleByteSource reads a stream a byte at a time and gives the
// read up when a trapped signal does, in the dialect that abandons a `read`
// that way, and stops it to run the handlers in the dialect that runs them in
// the middle of one. See Semantics.ReadIsAbandonedByATrappedSignal,
// Semantics.ReadAbandonedWhen and Semantics.ReadHandlersRun.
//
// A descriptor is waited on rather than read until its byte is there — see
// readWatch.awaitByte — so a read given up leaves nothing in flight. Only a
// stream that is not a descriptor is read on a goroutine, for
// timedByteSource's reason: an io.Reader cannot be told to stop waiting. A
// byte that arrives on one of those after the read was given up is lost,
// which is that same cost and is confined to the stream the abandoned read was
// on. There every byte is a wait, since nothing says whether it was already
// there, and a read that stops to run handlers goes back to waiting for the
// byte already asked for. stop releases the goroutine.
func (r *Runner) trapInterruptibleByteSource(in io.Reader, w *readWatch) (next func() (byte, int), stop func()) {
	if fd, can := pollableDescriptor(in); can {
		return w.track(polledSource(in, func() int {
			return w.awaitByte(context.Background(), fd, time.Time{})
		})), func() {}
	}
	type event struct {
		b   byte
		eof bool
	}
	req := make(chan struct{})
	var got event
	ready := make(chan chan struct{}, 1)
	go func() {
		for range req {
			done := make(chan struct{})
			ready <- done
			var b [1]byte
			n, err := in.Read(b[:])
			got = event{b: b[0], eof: n == 0 || err != nil}
			close(done)
			if got.eof {
				return
			}
		}
	}()
	next = w.track(func() (byte, int) {
		mark := r.arrivalMark()
		req <- struct{}{}
		done := <-ready
		for {
			w.ended = readGivenUp
			sig, trapped, _ := r.awaitOrArrival(done, nil, w.arrivedDuring(mark))
			if !trapped {
				break
			}
			if w.ended != readRunsHandlers {
				return 0, w.gaveUp(sig)
			}
			if w.runHandlers() {
				return 0, evEOF
			}
			mark = r.arrivalMark()
		}
		if got.eof {
			return 0, evEOF
		}
		return got.b, evByte
	})
	stop = func() { close(req) }
	return next, stop
}

// anySignalTrapped reports whether a signal this shell runs a handler for is
// set, which is the only thing that can interrupt a read.
func (r *Runner) anySignalTrapped() bool {
	for name, action := range r.trapTable() {
		if action != "" && name != "CHLD" {
			return true
		}
	}
	return false
}
