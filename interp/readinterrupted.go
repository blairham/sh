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

// trapInterruptibleByteSource reads a stream a byte at a time and gives up
// the read when a trapped signal arrives, in the dialect that abandons a
// `read` that way. See Semantics.ReadIsAbandonedByATrappedSignal.
//
// A descriptor is waited on rather than read until its byte is there — see
// pollingTrapSource — so a read given up leaves nothing in flight. Only a
// stream that is not a descriptor is read on a goroutine, for
// timedByteSource's reason: an io.Reader cannot be told to stop waiting. A
// byte that arrives on one of those after the read was given up is lost,
// which is that same cost and is confined to the stream the abandoned read was
// on. stop releases the goroutine.
//
// ok is false where nothing could interrupt the read: no signal is trapped,
// or the stream is a regular file, whose reads do not wait for anybody.
func (r *Runner) trapInterruptibleByteSource(in io.Reader) (next func() (byte, int), stop func(), ok bool) {
	if r.sem().ReadIsAbandonedByATrappedSignal != Yes || !r.anySignalTrapped() {
		return nil, nil, false
	}
	if f, isFile := in.(*os.File); isFile {
		if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
			return nil, nil, false
		}
	}
	if fd, can := pollableDescriptor(in); can {
		return r.pollingTrapSource(in, fd), func() {}, true
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
	finished := false
	next = func() (byte, int) {
		if finished {
			return 0, evEOF
		}
		req <- struct{}{}
		done := <-ready
		if sig, trapped, _ := r.awaitOrTrap(done, nil); trapped {
			// Given up, as at the end of the input: nothing is assigned
			// and the status is 1. The handler runs once the builtin has
			// returned, like any other trap between commands.
			finished = true
			r.readAbandonedBy = sig
			return 0, evEOF
		}
		if got.eof {
			finished = true
			return 0, evEOF
		}
		return got.b, evByte
	}
	stop = func() { close(req) }
	return next, stop, true
}

// pollingTrapSource is trapInterruptibleByteSource on a descriptor: each byte
// is read only once it is there, and the wait for it is what a trapped signal
// ends.
//
// **A read given up is not left parked on the stream.** The goroutine this
// replaces was in the middle of a read when the signal came, and that read
// went on waiting after the builtin had returned — so it took the next byte to
// arrive and threw it away, and the next `read` got the line without it.
// Measured 2026-10-29, a fifo on descriptor 3, an INT trap, and `late` written
// half a second after the signal:
//
//	exec 3<>"$1"; trap "echo T" INT
//	(sleep 0.3; kill -INT $$; sleep 0.5; echo late >"$1") &
//	read -r l <&3; echo "st=$? l=[$l]"; read -r m <&3; echo "next [$m]"
//
// dash 0.5.12, ksh93u+ and BusyBox ash 1.37.0 all give the read up and hand
// `late` to the next one; this shell answered `next [ate]` (#7024). The same
// holds with part of the line in before the signal: dash and ksh93 keep `ab`
// in the name, ash drops it, and in all three `cd`, written after, is the next
// read's whole.
//
// A signal already waiting when the next byte is asked for gives the read up
// there, whether or not the byte is in — the reading this source has always
// had, and ksh93's on a fifo, kept here as it was; dash and ash instead finish
// a line whose input is still arriving (#7027). What it no longer does is take
// that byte first.
func (r *Runner) pollingTrapSource(in io.Reader, fd int) func() (byte, int) {
	finished := false
	read := polledSource(in, func() int {
		if sig, trapped := r.awaitReadableOrTrap(fd); trapped {
			// Given up, as at the end of the input. The handler runs once
			// the builtin has returned, like any other trap between
			// commands.
			r.readAbandonedBy = sig
			return evEOF
		}
		return evByte
	})
	return func() (byte, int) {
		if finished {
			return 0, evEOF
		}
		b, ev := read()
		if ev != evByte {
			finished = true
		}
		return b, ev
	}
}

// awaitReadableOrTrap waits until fd has something to read or a trapped signal
// arrives, and reports the signal when one did.
//
// The waiting for a byte happens on a goroutine and the waiting for a signal
// in awaitOrTrap, which is what makes `wait` interruptible and already knows
// every way a signal reaches this shell. The goroutine only asks whether a
// byte is there and never reads one, so letting it go is all a give-up has to
// do: it notices within canceledReadNoticedWithin and returns having taken
// nothing.
//
// Where the byte is already there no goroutine is started; awaitOrTrap is
// handed a wait that has already ended, and still says so if a signal is
// waiting too.
func (r *Runner) awaitReadableOrTrap(fd int) (syscall.Signal, bool) {
	readable := make(chan struct{})
	if ready, _ := fdset.ReadableWithin(fd, 0); ready {
		close(readable)
	} else {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			defer close(readable)
			awaitReadable(ctx, fd, time.Time{})
		}()
	}
	sig, trapped, _ := r.awaitOrTrap(readable, nil)
	return sig, trapped
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
