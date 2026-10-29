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
// the dialect that gives one up. See Semantics.ReadIsAbandonedByATrappedSignal
// and Semantics.ReadAbandonedWhen.
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
	// firstWaitOnly is the watch of a `read -t` in a dialect whose reads go
	// on through a trapped signal. It watches the wait for the first byte and
	// no other, asks Semantics.ReadTimeoutWaitEndedByATrappedSignal rather
	// than Semantics.ReadAbandonedWhen, and ends the read as its deadline
	// would. ends is that answer, and asked says it has been asked.
	firstWaitOnly, ends bool
}

// readWatchFor is the watch for a read of in, and false where nothing can give
// that read up: the dialect reads on through a trapped signal, no signal is
// trapped, or the stream is a regular file, whose reads do not wait for
// anybody.
func (r *Runner) readWatchFor(in io.Reader, byNumber, counted bool) (*readWatch, bool) {
	if r.sem().ReadIsAbandonedByATrappedSignal != Yes || !r.anySignalTrapped() {
		return nil, false
	}
	if f, isFile := in.(*os.File); isFile {
		if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
			return nil, false
		}
	}
	return &readWatch{r: r, in: in, byNumber: byNumber, counted: counted, began: r.arrivalMark()}, true
}

// timedReadWatchFor is the watch for a `read -t` of in, nil where nothing can
// end that read early: readWatchFor's, in a dialect a trapped signal gives any
// read up in, and otherwise one that watches only the wait for the first byte.
// See Semantics.ReadTimeoutWaitEndedByATrappedSignal.
//
// The second is made wherever a signal is trapped, and asks nothing until one
// lands in that wait — so a timed read whose byte arrives in time, which is
// every timed read of an ordinary script, puts no question at all.
func (r *Runner) timedReadWatchFor(in io.Reader, byNumber, counted bool) *readWatch {
	if w, ok := r.readWatchFor(in, byNumber, counted); ok {
		return w
	}
	if r.sem().ReadIsAbandonedByATrappedSignal == Yes || !r.anySignalTrapped() {
		return nil
	}
	return &readWatch{r: r, in: in, firstWaitOnly: true}
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
	if w.firstWaitOnly {
		// A byte there is a wait that never began.
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

// arrivedDuring is the question a wait for the next byte asks each time
// something may have arrived, mark being arrivalMark as the wait began.
//
// A signal that came before the wait does not end it in the reading that
// gives a read up only during one — it is still on the pending list, and it
// runs once the read is over — so that reading looks only past the mark.
func (w *readWatch) arrivedDuring(mark int) func() (syscall.Signal, bool) {
	if w.firstWaitOnly {
		return w.endsTheFirstWait(mark)
	}
	return func() (syscall.Signal, bool) {
		sig, ok := w.r.pendingTrap()
		if !ok {
			return 0, false
		}
		switch w.readingNow() {
		case ReadAbandonedOnlyDuringAWait:
			return w.r.pendingTrapFrom(mark)
		case ReadAbandonedOnlyAtTheStartOfALine:
			if sig, gives, handled := w.startOfALine(&mark); handled {
				return sig, gives
			}
		}
		return sig, true
	}
}

// endsTheFirstWait is arrivedDuring for the watch that watches a timed read's
// wait for its first byte and no other: only a trapped signal that lands
// during that wait is looked at, and the dialect is asked about it then. A
// signal handled before the read began is not one — measured, zsh reads on
// over it — and nor is one already waiting, which pendingTrapFrom leaves out.
//
// An unanswered axis is refused there, and the read ends so that the builtin
// can report the refusal.
func (w *readWatch) endsTheFirstWait(mark int) func() (syscall.Signal, bool) {
	return func() (syscall.Signal, bool) {
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
}

// gaveUp records that sig gave the read up and ends it as at the end of the
// input: nothing more is read, and the handler runs once the builtin has
// returned, like any other trap between commands.
//
// The watch of a timed read's first wait ends it as the deadline would
// instead, which is what that reading is — the status is the timeout's and no
// name is touched — and leaves the read's status nothing of the signal's.
func (w *readWatch) gaveUp(sig syscall.Signal) int {
	if w.firstWaitOnly {
		return evTimeout
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
func (w *readWatch) awaitByte(ctx context.Context, fd int, deadline time.Time) int {
	if ready, _ := fdset.ReadableWithin(fd, 0); ready {
		if sig, gives := w.givesUpWithAByteThere(); gives {
			return w.gaveUp(sig)
		}
		return evByte
	}
	mark := w.r.arrivalMark()
	readable := make(chan struct{})
	var ready, can bool
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		defer close(readable)
		ready, can = awaitReadable(ctx, fd, deadline)
	}()
	if sig, trapped, _ := w.r.awaitOrArrival(readable, nil, w.arrivedDuring(mark)); trapped {
		return w.gaveUp(sig)
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

// trapInterruptibleByteSource reads a stream a byte at a time and gives the
// read up when a trapped signal does, in the dialect that abandons a `read`
// that way. See Semantics.ReadIsAbandonedByATrappedSignal and
// Semantics.ReadAbandonedWhen.
//
// A descriptor is waited on rather than read until its byte is there — see
// readWatch.awaitByte — so a read given up leaves nothing in flight. Only a
// stream that is not a descriptor is read on a goroutine, for
// timedByteSource's reason: an io.Reader cannot be told to stop waiting. A
// byte that arrives on one of those after the read was given up is lost,
// which is that same cost and is confined to the stream the abandoned read was
// on. There every byte is a wait, since nothing says whether it was already
// there. stop releases the goroutine.
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
		if sig, trapped, _ := r.awaitOrArrival(done, nil, w.arrivedDuring(mark)); trapped {
			return 0, w.gaveUp(sig)
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
