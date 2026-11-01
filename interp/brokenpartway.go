// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"syscall"
)

// A write into a pipe whose reader leaves part-way through can raise SIGPIPE
// twice, not once.
//
// The kernel raises the signal for each write(2) that finds the reader gone.
// A pipeline's write end is a blocking descriptor here (see raiseShellEnd), so
// a write bigger than the pipe can take copies what fits and sleeps. If the
// reader leaves then, Linux hands back the short count *and* raises SIGPIPE.
// os.File.Write treats a short count as "write the rest" and calls write(2)
// again. That call finds no reader, returns EPIPE and raises a second SIGPIPE.
// Measured 2026-11-01 in ubuntu:26.04 on a bare blocking pipe whose reader
// closed mid-write: n=8192 with one SIGPIPE, then EPIPE with a second. A write
// that finds the reader already gone raises exactly one.
//
// The shell answers the signal at the write and takes the runtime's copies out
// of the way with a credit for each (see Runner.writeSignalAbsorbed). One
// credit per failed write was one too few for the part-way case. The runtime
// usually merges the two signals into one pending bit, but when its forwarding
// goroutine picks up the first before the retry raises the second, both reach
// the channel. The spare copy was then taken for a PIPE from outside, and the
// shell around a pipeline element ran its own handler for the element's broken
// pipe. Measured with a race build of the interp tests in Linux: 5 of 3000 runs
// of the two element tests in ignoredpipe_test.go ran the outer handler. Every
// one of the 5 came after a write that had gone 8192 bytes before failing, and
// none came after the 2463 writes that failed at 0 bytes (#7201). macOS
// delivers no SIGPIPE for these writes at all, which is why only the Linux leg
// ever saw it.
//
// So a failed write records whether any of it went through, and a broken pipe
// that went part-way is credited for two copies. Where the runtime did merge
// them, the second credit is the one nobody spends, which costs what an unspent
// credit always has: one later copy of the same signal from outside the process.

// pipeBrokenPartWay is a broken-pipe error from a write that had already put
// some of its bytes into the pipe. It reads as the error it wraps, so the
// wording of the failure and every errors.Is on it are unchanged.
type pipeBrokenPartWay struct{ err error }

func (e pipeBrokenPartWay) Error() string { return e.err.Error() }
func (e pipeBrokenPartWay) Unwrap() error { return e.err }

// failedPartWay is the error to record for a write that wrote n bytes and then
// failed with err: marked when the pipe broke after part of it went through,
// and err unchanged otherwise.
func failedPartWay(n int, err error) error {
	if n > 0 && errors.Is(err, syscall.EPIPE) {
		return pipeBrokenPartWay{err}
	}
	return err
}

// kernelPipeCopies is how many SIGPIPEs a failed write with err may have had
// the kernel raise: two where the pipe broke part-way, one otherwise.
func kernelPipeCopies(err error) int {
	var partWay pipeBrokenPartWay
	if errors.As(err, &partWay) {
		return 2
	}
	return 1
}
