// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
)

// trapInterruptibleByteSource reads a stream a byte at a time and gives up
// the read when a trapped signal arrives, in the dialect that abandons a
// `read` that way. See Semantics.ReadIsAbandonedByATrappedSignal.
//
// The reads happen on their own goroutine, for timedByteSource's reason: an
// io.Reader cannot be told to stop waiting. A byte that arrives after the
// read was given up is lost, which is that same cost and is confined to the
// stream the abandoned read was on. stop releases the goroutine.
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
