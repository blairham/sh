// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package fdset_test

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/blairham/sh/internal/fdset"
)

// A wait cut short by a signal must be asked again, not answered as "nothing
// is ready".
//
// [fdset.Wait] is the call an editor sits in at an idle prompt with a watcher
// armed, and its answer decides what happens next: an empty one means *go and
// read a key*, which is a blocking read on the terminal and not on the
// descriptor. So a `select` interrupted by a signal, reported as an empty
// result, abandons the watcher until somebody presses something — which is
// zsh-autosuggestions' async suggestion arriving on the next keystroke
// instead of on its own (#4413).
//
// **The signal is SIGURG, and that choice is the measurement rather than a
// convenience.** Measured 2026-09-24 on darwin against this call: SIGUSR1,
// SIGWINCH and SIGCHLD are all restarted by the kernel and never produce
// EINTR here, so a test written with any of them **passes against the broken
// code** — it was, and it did. SIGURG is the one that interrupts, and it is
// also the one that caused the bug in the field, because it is the Go
// runtime's own goroutine-preemption signal and so arrives unbidden at any
// prompt.
//
// It needs no signal.Notify: the runtime already handles SIGURG, which is the
// whole reason it reaches a blocking select.
func TestAWaitCutShortByASignalIsAskedAgain(t *testing.T) {
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = read.Close(); _ = write.Close() })

	// A terminal that never becomes readable, so the only thing that can end
	// the wait is the descriptor. Without one Wait polls instead of blocking,
	// and the interruption it is about could not happen.
	terminal, terminalWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = terminal.Close(); _ = terminalWrite.Close() })

	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			_ = syscall.Kill(os.Getpid(), syscall.SIGURG)
			time.Sleep(time.Millisecond)
		}
	}()
	// Well after the signals have started, so a Wait that gives up on the
	// first interruption answers empty long before there is anything to
	// report — and does so almost immediately, which is what the timing
	// assertion below reads.
	go func() {
		time.Sleep(300 * time.Millisecond)
		_, _ = write.WriteString("wake\n")
	}()

	start := time.Now()
	ready, terminalReady, err := fdset.Wait(int(terminal.Fd()), []int{int(read.Fd())})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if terminalReady {
		t.Fatal("the terminal was reported ready; nothing was ever written to it")
	}
	if len(ready) != 1 || ready[0] != int(read.Fd()) {
		t.Fatalf("ready = %v after %v, want the one descriptor %d — a signal mid-wait was "+
			"answered as 'nothing is ready', which is what abandons an armed watcher",
			ready, elapsed.Round(time.Millisecond), int(read.Fd()))
	}
	// And it really waited rather than happening to be asked after the write.
	if elapsed < 250*time.Millisecond {
		t.Errorf("Wait returned after %v, before the descriptor was written to at 300ms — "+
			"it cannot have waited through the signals", elapsed.Round(time.Millisecond))
	}
}
