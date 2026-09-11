// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package interp

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// nudgeFifoEOF has three ways out and all of them are load-bearing: a loop
// with none would run for the life of the process, and a loop missing one
// would run for the life of whatever the missing answer was going to end it.
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
		// at the end of the command that named it.
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
// contract: that it ends, on each of the two answers that end it.

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
// read together: `>(cmd)` has never needed nudging because the shell is its
// reader and holds the pipe open across the whole command.
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
