// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/blairham/sh/interp"
)

// A fatal signal that arrives from outside has to end this shell the way it
// ends every other one, and the Go runtime will not do it.
//
// dieBySignal covers the signal a script sends *itself*: it puts the kernel's
// default action back before raising, so `kill -QUIT $$` kills the shell
// silently. A signal from another process takes a different road and never
// reaches that code at all — the runtime's own handler is still installed, and
// what it does depends on a classification we get no say in. Signals it calls
// killing, HUP and INT and TERM, it forwards to the default action, which is
// what a shell wants and is why those already work. Signals it calls throwing
// — QUIT, ABRT, ILL, TRAP, SYS, SEGV, BUS, FPE — it turns into a goroutine
// dump on standard error and an exit status of 2.
//
// Measured against the panel, with the shell in the foreground so nothing has
// inherited an ignore: bash, dash and zsh all die of every one of those eight,
// print nothing at all, and leave 128 plus the number. This shell printed a
// page of Go and exited 2 for all eight. There is no disagreement to model and
// no dialect answer to ask for; it is one behavior, and it is the last road
// for the leak internal/panicguard exists to close.
//
// So the front end listens. os/signal is what turns the runtime's handler into
// a delivery this program can act on, and acting on it is dieBySignal — the
// same restore-and-raise the self-signal path already uses, so the two roads
// end in the same place.
//
// On every platform, which it was not: darwin had a file of its own saying the
// front end must not listen there, because asking os/signal for a signal makes
// that runtime open a *pipe* to carry it, on descriptors low enough for a
// script to name — and `exec 3>f` then wrote over the runtime's own end of it,
// killing the shell where it was about to become the command. That was #799.
// driver/lowfds_unix.go keeps the runtime above every number a script can
// name, so the pipe is out of reach and this file has no exception left to
// make. Five of the eight arrive on darwin rather than all eight, for a
// reason that is the kernel's and not ours; driver/die_test.go's
// reachesTheFrontEnd has it measured.

// fatalSignals are the ones the Go runtime treats as throwing: it prints a
// goroutine dump and exits rather than letting the default action run.
//
// KILL is not among them and could not be: nothing catches it, and nothing
// needs to, since the kernel never lets the runtime see it. The killing class
// — HUP, INT, TERM and the rest — is not here either, because the runtime
// already forwards those to the default action, and taking them over would
// mean reimplementing what already works.
var fatalSignals = []os.Signal{
	syscall.SIGQUIT, syscall.SIGILL, syscall.SIGTRAP, syscall.SIGABRT,
	syscall.SIGBUS, syscall.SIGFPE, syscall.SIGSEGV, syscall.SIGSYS,
}

// fatalWatch is this process's own handling of those signals.
//
// One for the process, and that is the honest model rather than a shortcut: a
// signal disposition belongs to the process, there is one process, and a
// second registration would be a second goroutine racing the first to end it.
// The shell it asks about is a pointer that moves, because a front end builds
// a Runner per invocation and a test binary builds hundreds — what matters is
// that the question reaches whichever shell is running now.
var fatalWatch struct {
	once sync.Once
	// traps answers whether the current shell has a trap for a signal. Nil
	// until a shell that owns the process has been built.
	traps atomic.Pointer[func(syscall.Signal) bool]
}

// watchFatalSignals arranges for an untrapped fatal signal from outside to end
// this process the way it ends any other shell.
//
// Called only where the front end owns the process — the same condition as
// ReplaceProcess and DieBySignal, and for the same reason. A Runner embedded in
// another program must not take that program's signal handling away from it on
// the say-so of a shell it happens to be running.
func watchFatalSignals(r *interp.Runner) {
	traps := r.TrapsSignal()
	fatalWatch.traps.Store(&traps)
	fatalWatch.once.Do(func() {
		// Buffered, because os/signal drops an arrival rather than blocking
		// and there are eight of these. Nothing here is slow, but a shell
		// being killed is not the moment to depend on that.
		ch := make(chan os.Signal, len(fatalSignals))
		signal.Notify(ch, fatalSignals...)
		go func() {
			for s := range ch {
				sig, ok := s.(syscall.Signal)
				if !ok {
					continue
				}
				if traps := fatalWatch.traps.Load(); traps != nil && (*traps)(sig) {
					// The script has claimed it, and interp's own
					// registration has been handed the same arrival. Two
					// channels are told; only one of them may act.
					continue
				}
				// Untrapped, so the shell dies of it — silently, with the
				// status a shell killed by a signal leaves, which is what
				// dieBySignal is already for. It does not return.
				_ = dieBySignal(sig)
			}
		}()
	})
}
