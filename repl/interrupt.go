// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// catchInterrupt keeps ^C from killing the shell while a command runs.
//
// At the prompt the terminal is raw and ISIG is off, so ^C is a byte the
// editor sees and no signal is sent at all. While a command runs the terminal
// is back in its own line discipline, ^C reaches the whole foreground process
// group, and this shell is in it — so without this, stopping a runaway command
// stops the shell with it.
//
// Caught rather than ignored. The distinction is not a detail: POSIX carries an
// *ignored* disposition across exec, so setting SIGINT to SIG_IGN here would
// leave every child ignoring it too and ^C would stop nothing at all. A handler
// is reset to the default in the child, so the child dies and the shell does
// not.
//
// The signals are drained and discarded. What the shell has to do about an
// interrupted command it learns from the command's status — 128 plus the
// signal — rather than from hearing the signal itself, and the trap machinery
// in interp is what a script's own `trap INT` goes through.
// The channel is also what tells the loop to start the next prompt on a fresh
// line: the terminal echoes `^C` where the cursor was and does not move it, so
// without a newline the prompt lands on top of it.
type interrupts struct {
	ch  chan os.Signal
	hit atomic.Bool
}

func catchInterrupt() (*interrupts, func()) {
	in := &interrupts{ch: make(chan os.Signal, 1)}
	signal.Notify(in.ch, syscall.SIGINT)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-in.ch:
				in.hit.Store(true)
			case <-done:
				return
			}
		}
	}()
	return in, func() {
		signal.Stop(in.ch)
		close(done)
	}
}

// took reports whether a ^C arrived since it was last asked, and forgets it.
func (in *interrupts) took() bool { return in.hit.Swap(false) }
