// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// catchInterrupt keeps ^C and ^Z from reaching the shell while a command runs.
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
// Two flags rather than one, because two things read an arrival and neither
// may take it from the other. hit is the prompt's: the terminal echoed `^C`
// where the cursor was, so the next prompt starts on a line of its own.
// pending is the interpreter's, read through Runner.TakeInterrupt, and is what
// stops a loop the shell is running itself — the one thing nothing else can
// reach, since a loop of builtins never blocks, never waits and never comes
// back here.
type interrupts struct {
	ch      chan os.Signal
	hit     atomic.Bool
	pending atomic.Bool
}

func catchInterrupt() (*interrupts, func()) {
	in := &interrupts{ch: make(chan os.Signal, 1)}
	// SIGTSTP for the same reason as SIGINT, and with the same distinction:
	// caught, so the *job* stops and the shell does not. Ignoring it would be
	// inherited across exec and nothing would ever stop.
	signal.Notify(in.ch, syscall.SIGINT, syscall.SIGTSTP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case sig := <-in.ch:
				in.hit.Store(true)
				if offersToTheInterpreter(sig) {
					in.pending.Store(true)
				}
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

// take is the same question asked by the interpreter, through
// Runner.TakeInterrupt, and answered from a flag of its own so that the two
// readers cannot take an arrival from each other.
func (in *interrupts) take() bool { return in.pending.Swap(false) }

// offersToTheInterpreter reports whether an arrival is one the interpreter
// should be told about: an interrupt, and not a stop.
//
// A stop is not an interrupt. What ^Z does to a loop is decided from the
// command that stopped — see interp's breakLoopsForAStop, and the measurements
// behind it — and a shell that gave the line up here as well would end the
// line that bash and zsh both carry on with.
func offersToTheInterpreter(sig os.Signal) bool { return sig == syscall.SIGINT }
