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
	// ReadAbandonedOnlyAtTheStartOfALine gives the read up only while it is
	// waiting for the first byte of a line — at its start, or just after a
	// newline it has read, which is where a continued line or a `-d` record
	// with a newline in it starts another. Once a byte of the line is in,
	// the read goes on to the line's end, the deadline of a `-t` still
	// bounding it, and no signal gives it up. ksh93.
	//
	// Only on a read of the builtin's standard input, from a descriptor open
	// for reading alone, that reads to a delimiter. A read given its stream
	// by `-u` or `-p`, one from a descriptor open for writing as well — `exec
	// 3<>fifo` — and one that counts with `-n` or `-N` are given up as
	// ReadAbandonedAtTheNextByte gives them up.
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
	// covered is whether ReadAbandonedOnlyAtTheStartOfALine reaches this
	// read, worked out the first time it matters: 0 not yet, 1 yes, -1 no.
	covered int8
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
	return &readWatch{r: r, in: in, byNumber: byNumber, counted: counted}, true
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

// keepsTheLine reports that the read has a line under way that no signal now
// gives up, which is ReadAbandonedOnlyAtTheStartOfALine's case.
func (w *readWatch) keepsTheLine() bool {
	if !w.inLine || w.byNumber || w.counted {
		return false
	}
	if w.covered == 0 {
		w.covered = 1
		if f, ok := w.in.(interface{ Fd() uintptr }); ok && descriptorOpenForWriting(int(f.Fd())) {
			w.covered = -1
		}
	}
	return w.covered > 0
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
	sig, ok := w.trapWaiting()
	if !ok {
		return 0, false
	}
	switch w.readingNow() {
	case ReadAbandonedOnlyDuringAWait:
		return 0, false
	case ReadAbandonedOnlyAtTheStartOfALine:
		if w.keepsTheLine() {
			return 0, false
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
	return func() (syscall.Signal, bool) {
		sig, ok := w.r.pendingTrap()
		if !ok {
			return 0, false
		}
		switch w.readingNow() {
		case ReadAbandonedOnlyDuringAWait:
			return w.r.pendingTrapFrom(mark)
		case ReadAbandonedOnlyAtTheStartOfALine:
			if w.keepsTheLine() {
				return 0, false
			}
		}
		return sig, true
	}
}

// gaveUp records that sig gave the read up and ends it as at the end of the
// input: nothing more is read, and the handler runs once the builtin has
// returned, like any other trap between commands.
func (w *readWatch) gaveUp(sig syscall.Signal) int {
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
