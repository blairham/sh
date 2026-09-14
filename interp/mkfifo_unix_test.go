// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// nudgeFifoEOF has three ways out and all of them are load-bearing: a loop
// with none would run for the life of the process, and a loop missing one
// would run for the life of whatever the missing answer was going to end it.
//
// Two of them carry the ordinary case between them, and which one does has
// moved: since #2733 the shell holds a reading end of a `<(cmd)`'s pipe for
// the life of the command, so the first subtest's answer is what a *finished*
// substitution gives and the second's is what a running one gives. Both are
// still reachable and both still end the loop, which is what these check.
func TestNudgingAPipeEndsOnBothOfItsAnswers(t *testing.T) {
	t.Run("no reader left to tell", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sub")
		if err := mkfifo(path); err != nil {
			t.Fatal(err)
		}
		// Nobody has it open for reading, so the very first open answers
		// ENXIO and there is nothing to do.
		if !within(t, time.Second, func() { nudgeFifoEOF(path) }) {
			t.Error("nudging a pipe nobody reads did not stop")
		}
	})

	t.Run("no pipe to tell through", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sub")
		if err := mkfifo(path); err != nil {
			t.Fatal(err)
		}
		// A reader is present, so the opens succeed and the loop goes round
		// — until the pipe is taken away, which is what removeProcSubs does
		// at the end of the command that named it. That is also where the
		// shell's own reading end is released, so this is the answer an
		// ordinary `<(cmd)` ends on.
		end, hold, err := openFifoReadEnd(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = end.Close(); _ = hold.Close() }()
		go func() {
			time.Sleep(50 * time.Millisecond)
			_ = os.Remove(path)
		}()
		if !within(t, 5*time.Second, func() { nudgeFifoEOF(path) }) {
			t.Error("nudging did not stop when the pipe was taken away")
		}
	})

	t.Run("a reader that stays, and a pipe that stays with it", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "sub")
		if err := mkfifo(path); err != nil {
			t.Fatal(err)
		}
		// The arrangement #1750 made possible and #1907 was: the reader is
		// one of this shell's own descriptors, so the pipe keeps its name
		// for as long as the descriptor is open — and neither of the other
		// two answers can ever arrive. Measured before the deadline went
		// in: 56 rounds in the second of script life after the body had
		// finished, and no end to it but the process exiting.
		end, hold, err := openFifoReadEnd(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = end.Close(); _ = hold.Close() }()
		if !within(t, 5*time.Second, func() { nudgeFifoEOF(path) }) {
			t.Error("nudging a pipe whose reader stays did not stop")
		}
	})
}

// There is deliberately no unit test here that the nudge *delivers* an
// end-of-file, and the reason is worth writing down rather than leaving as an
// absence. The delivery is only observable in the state the race produces —
// a reader parked on a pipe whose end-of-file has already gone past — and
// that state cannot be built to order: a reader that is not parked takes the
// end-of-file from the first close, and a reader that has not opened yet
// makes the first nudge answer ENXIO and stop. An attempt at one raced its
// own reader and asserted nothing.
//
// What covers the delivery is the stress case,
// TestAProcessSubstitutionAlwaysDeliversItsEndOfFile, against a measured
// failure rate on the unfixed tree. What is covered here is the loop's own
// contract: that it ends, on each of the answers that end it.

// The writing end's placeholder is the other half of #2733, and unlike the
// nudge it *can* be asked directly, because it is a descriptor rather than a
// repetition. Its two halves: it has to keep the pipe alive, and it must not
// keep an end-of-file away. See openFifoWriteEnd for the measurement that put
// it there, and for why it does not make the nudge above unnecessary.
func TestTheWriteEndsPlaceholderKeepsThePipeOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub")
	if err := mkfifo(path); err != nil {
		t.Fatal(err)
	}
	// A reader, standing in for the command that was given the path: the
	// write end's open is a poll for exactly this.
	rfd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	end, hold, err := openFifoWriteEnd(path)
	if err != nil {
		t.Fatalf("opening the write end against a reader: %v", err)
	}
	defer func() { _ = hold.Close() }()
	if _, err := end.Write([]byte("hi")); err != nil {
		t.Fatalf("writing the body's output: %v", err)
	}
	// The body is done and the command has gone — which on a pipe with no
	// placeholder is the last reader and the last writer leaving together,
	// and the buffer going with them.
	_ = end.Close()
	_ = syscall.Close(rfd)
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("the pipe was not held open after both real ends closed: %v", err)
	}
	_ = syscall.Close(fd)
}

// And the half that says why it is O_RDONLY. The reading end's placeholder is
// O_RDWR because a body reading a `>(cmd)` must not see an immediate
// end-of-file; this one is the mirror image and must not *prevent* one, since
// the shell's writing end closing is the only end-of-file a `<(cmd)`'s command
// is ever going to get.
func TestTheWriteEndsPlaceholderStillLetsTheEndOfFileThrough(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub")
	if err := mkfifo(path); err != nil {
		t.Fatal(err)
	}
	rfd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Read as the substituted command reads: blocking, so that a pipe with a
	// writer still in it is a read that waits rather than one that fails.
	if err := syscall.SetNonblock(rfd, false); err != nil {
		t.Fatal(err)
	}
	reader := os.NewFile(uintptr(rfd), path)
	defer func() { _ = reader.Close() }()
	end, hold, err := openFifoWriteEnd(path)
	if err != nil {
		t.Fatalf("opening the write end against a reader: %v", err)
	}
	defer func() { _ = hold.Close() }()
	if _, err := end.Write([]byte("hi")); err != nil {
		t.Fatalf("writing the body's output: %v", err)
	}
	_ = end.Close()
	var got []byte
	if !within(t, 5*time.Second, func() { got, _ = io.ReadAll(reader) }) {
		t.Fatal("reading to end-of-file with the placeholder held did not finish")
	}
	if string(got) != "hi" {
		t.Errorf("read %q, want %q", got, "hi")
	}
}

// within runs body and reports whether it finished in time. The body is left
// running if it did not, which is the same trade deadline makes and for the
// same reason: nothing here can stop a loop in a syscall.
func within(t *testing.T, d time.Duration, body func()) bool {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		body()
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// The reading end's placeholder is what stops the writing end waiting, and is
// the other direction's answer to the same question. Held here so the two are
// read together: this is the one `>(cmd)` has always had, and the pair above
// is what `<(cmd)` was missing until #2733.
func TestTheReadEndsPlaceholderKeepsThePipeOpen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub")
	if err := mkfifo(path); err != nil {
		t.Fatal(err)
	}
	end, hold, err := openFifoReadEnd(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = end.Close() }()
	// With the placeholder open, a writer's open does not wait.
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatalf("opening the write end with the placeholder held: %v", err)
	}
	_ = syscall.Close(fd)
	_ = hold.Close()
	_ = os.Remove(path)
}
