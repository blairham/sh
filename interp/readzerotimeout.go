// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "io"

// ReadZeroTimeoutStyle is what `read -t 0` asks of the stream.
//
// A timeout of zero is not a small timeout. Every dialect with the letter
// treats it as a question about the *state* of the input rather than as a
// deadline that has already passed, and the panel gives it three answers —
// measured with a line already in the pipe, with nothing there at all, and
// with a partial line waiting and the rest of it half a second away:
//
//	                     line waiting   nothing waiting   `ab` waiting
//	poll                 0, nothing read   1               0, nothing read
//	take what is waiting 0, line read      1               1, nothing kept
//	finish what it starts 0, line read     1               0, `abc` read
//
// The first column is the same everywhere and the second is too, which is why
// this looked like one behavior for a long time. It is three, and the third
// column separates them.
//
// The distinction that matters to a script is the first: a shell that polls
// consumes nothing, so `while ! read -t 0; do …; done` asks the same question
// every time round. A shell that reads what is waiting answers the question by
// taking the input, so the same loop eats what it was watching for.
type ReadZeroTimeoutStyle int

const (
	// ReadZeroTimeoutUnspecified is no answer, and is refused like any other.
	ReadZeroTimeoutUnspecified ReadZeroTimeoutStyle = iota
	// ReadZeroTimeoutPolls answers whether a read would find something
	// without waiting, and reads nothing: status 0 when input is waiting or
	// the stream has ended, 1 when a read would have to wait. No variable is
	// assigned in either direction, not even cleared. bash.
	//
	// The status is 1 and not the number an expired `read -t` reports,
	// which is a second reason this is not a timeout: measured, the shell
	// that polls answers 1 here and 142 for a deadline that ran out.
	ReadZeroTimeoutPolls
	// ReadZeroTimeoutTakesWhatIsWaiting reads, but only through input that
	// is already there: the stream is asked again before every byte, so a
	// read that has begun still gives up the moment the input runs dry, and
	// what it had gathered is discarded like any other expired read. ksh93.
	ReadZeroTimeoutTakesWhatIsWaiting
	// ReadZeroTimeoutFinishesWhatItStarted asks once, before the first byte,
	// and then reads as an untimed read does — so a partial line waiting is
	// enough to commit it to waiting for the rest. zsh.
	ReadZeroTimeoutFinishesWhatItStarted
)

func (s ReadZeroTimeoutStyle) String() string {
	switch s {
	case ReadZeroTimeoutPolls:
		return "ReadZeroTimeoutPolls"
	case ReadZeroTimeoutTakesWhatIsWaiting:
		return "ReadZeroTimeoutTakesWhatIsWaiting"
	case ReadZeroTimeoutFinishesWhatItStarted:
		return "ReadZeroTimeoutFinishesWhatItStarted"
	}
	return "ReadZeroTimeoutUnspecified"
}

// pollingByteSource reads only input that is already waiting.
//
// everyByte asks the stream again before each byte; otherwise only the first
// byte is asked about and the rest is read as an ordinary read would. No
// goroutine and no deadline: the whole of the waiting is the question, which
// is answered before the read rather than raced against it.
func pollingByteSource(in io.Reader, everyByte bool) func() (byte, int) {
	direct := directByteSource(in)
	asked := false
	return func() (byte, int) {
		if everyByte || !asked {
			asked = true
			if !inputWaiting(in) {
				return 0, evTimeout
			}
		}
		return direct()
	}
}

// zeroTimeoutSource is the byte source for `read -t 0` under the two styles
// that read at all, and reports whether the style was one of them.
func (r *Runner) zeroTimeoutSource(in io.Reader) (func() (byte, int), bool) {
	switch r.sem().ReadZeroTimeout {
	case ReadZeroTimeoutTakesWhatIsWaiting:
		return pollingByteSource(in, true), true
	case ReadZeroTimeoutFinishesWhatItStarted:
		return pollingByteSource(in, false), true
	case ReadZeroTimeoutPolls, ReadZeroTimeoutUnspecified:
	}
	return nil, false
}
