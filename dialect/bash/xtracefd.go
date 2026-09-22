// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"io"
	"strconv"

	"github.com/blairham/sh/interp"
)

// `BASH_XTRACEFD`: where this shell writes its `set -x` trace.
//
// A parameter and not an option, and that is what makes it worth having: a
// script that wants its own trace has nowhere else to put it, since `set -x`
// otherwise shares standard error with everything the commands themselves
// say. `exec 4>trace; BASH_XTRACEFD=4` parts the two.
//
// Three things about it are measured rather than assumed, on bash 5.3.20 from
// a script file on 2026-09-22:
//
//   - The **assignment** is what complains, not the first trace line. `4` with
//     nothing open there writes `BASH_XTRACEFD: 4: invalid value for trace
//     file descriptor` at the assignment's own line, and a shell that waited
//     until something was traced would report it somewhere else or — with
//     `set -x` never turned on — not at all.
//   - It complains and **still assigns**: `echo $BASH_XTRACEFD` after the
//     refusal is `4`. So this is a complaint about the descriptor and not a
//     refused store, which is why it hangs off an assignment action rather
//     than off a producer.
//   - A number that *is* open but cannot be written draws a different
//     sentence: with `exec 5</dev/null`, `BASH_XTRACEFD=5` is `5: cannot open
//     as FILE`. An empty value is silent and puts the trace back on standard
//     error.
//
// The second sentence is reached from this shell's **own table** and not from
// the kernel, which is narrower than bash and is said here rather than left to
// be discovered: a number holding something this package cannot write at all —
// standard input, an embedder's reader, a here-document's text — is `cannot
// open as FILE`, and a number holding a file the script opened `<` is
// accepted, where bash asks the descriptor its mode and refuses it. Asking
// would mean a `fcntl` through Runner.SystemDescriptor, and the trade is that
// the narrow reading is portable and needs no platform file.
//
// Where the trace goes is read per line rather than resolved here, because the
// descriptor is a number in the script's own table and `exec 4>f` may come
// after the assignment — see interp.Runner.SetTraceSink.
const traceFdName = "BASH_XTRACEFD"

func registerTraceDescriptor(r *interp.Runner) {
	r.SetTraceSink(func(rr *interp.Runner) io.Writer {
		w, _ := traceDescriptor(rr)
		return w
	})
	r.SetAssignmentAction(traceFdName, func(rr *interp.Runner, value string) {
		switch _, why := traceDescriptorFor(rr, value); why {
		case traceFdFine:
		case traceFdNotADescriptor:
			rr.Diagnosef("%s: %s: invalid value for trace file descriptor\n", traceFdName, value)
		case traceFdNotWritable:
			rr.Diagnosef("%s: %s: cannot open as FILE\n", traceFdName, value)
		}
	})
}

// traceFdVerdict is what a value of the parameter is worth.
type traceFdVerdict int

const (
	// traceFdFine is a value the trace can be written to, and an empty one,
	// which is not a complaint and not a stream either.
	traceFdFine traceFdVerdict = iota
	// traceFdNotADescriptor is text that is no number, or a number nothing is
	// open at.
	traceFdNotADescriptor
	// traceFdNotWritable is a number this shell holds open for reading.
	traceFdNotWritable
)

// traceDescriptor is the stream the trace goes to now, and nil for standard
// error — which is both "no parameter" and "a parameter naming nothing", since
// a trace that cannot be written where it was sent is still written.
func traceDescriptor(r *interp.Runner) (io.Writer, traceFdVerdict) {
	value, ok := r.GetVar(traceFdName)
	if !ok {
		return nil, traceFdFine
	}
	return traceDescriptorFor(r, value)
}

func traceDescriptorFor(r *interp.Runner, value string) (io.Writer, traceFdVerdict) {
	if value == "" {
		return nil, traceFdFine
	}
	fd, err := strconv.Atoi(value)
	if err != nil || fd < 0 {
		return nil, traceFdNotADescriptor
	}
	if w, ok := r.WriterForFd(fd); ok && w != nil {
		return w, traceFdFine
	}
	if _, open := r.ReaderForFd(fd); open {
		// Open, and not something to write on: standard input, the reading
		// end of a pipe, a file opened `<`. Measured on bash 5.3.20 with
		// `exec 5</dev/null`, and on descriptor 0 with the shell's input on
		// a pipe — `5: cannot open as FILE` for both, which is a different
		// sentence from a number nothing is open at.
		return nil, traceFdNotWritable
	}
	return nil, traceFdNotADescriptor
}
