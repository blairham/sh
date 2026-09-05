// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package panicguard turns a panic raised while running a script into an
// ordinary diagnostic.
//
// interp panics when an invariant of its own breaks, and it is right to: it is
// a library, and a library that swallows a broken invariant hands its caller a
// wrong answer where it could have handed back nothing at all. A shell cannot
// answer that way, because the process *is* the session — a panic on one typed
// line takes a shell that has been open for hours, with its variables, its
// functions, its jobs and its directory — and the audience for this one is
// doubly embedders, whose host program must not die because a script tickled
// an interpreter bug.
//
// So the guard lives out here, in the packages that have already decided the
// process is a shell. It is the same split as ReplaceProcess and DieBySignal:
// interp declines to make the decision a library may not make, and the front
// end makes it. Nothing under interp/ recovers, and nothing here is reachable
// from a Runner an embedder drives itself — an embedder that wants the same
// guarantee wraps its own call, which is a `defer` it can write and a policy
// it can choose.
//
// The stack trace is deliberately behind an environment variable. A trace
// printed at a prompt scrolls the session away and buries the one line that
// says what happened, which is its own damage on top of the bug; asking for it
// is a decision a person makes once they know there is something to look at.
//
// Two limits are worth stating rather than discovering. A recover only reaches
// the goroutine that panicked, so a panic on a background job's own goroutine
// still ends the process; and a runtime failure the language does not make
// recoverable — a concurrent map write, a stack overflow — is not caught here
// either. The guard is a backstop for the common case, which is a bug on the
// line the shell is running right now.
package panicguard

import (
	"fmt"
	"io"
	"runtime/debug"
)

// TraceVar names the environment variable that asks for the stack trace of a
// caught panic. Any non-empty value asks for it.
//
// It is read by whoever builds a Guard and never here, which is the point of
// naming it in a constant: this package is used from repl, and repl is a
// library in the same sense interp is, so reaching for the process's
// environment inside it would be the leak the purity rule exists to stop.
// driver is where the process is legitimately visible and driver is where the
// variable is read.
const TraceVar = "SH_PANIC_TRACE"

// tracker is where a caught bug is reported, named in the diagnostic because
// the person reading it is the only one who saw it happen.
const tracker = "https://github.com/blairham/sh/issues"

// Status is what a run that panicked leaves behind.
//
// Nonzero, because nothing about that run succeeded, and 2 because that is
// what this front end already exits with when the *shell* failed rather than a
// command it ran. An internal error is the shell failing in the most literal
// way there is.
const Status = 2

// Guard runs work that may panic and reports the panic as a diagnostic.
//
// The zero value is usable and reports nothing, which is what a caller that
// has not said where diagnostics go should get — it still stops the panic.
type Guard struct {
	// Name is what the shell calls itself in the diagnostic, as it does in
	// every other one.
	Name string
	// Err is where the diagnostic goes. Nil discards it.
	Err io.Writer
	// Trace prints the stack of the panic as well as the one-line report.
	Trace bool
}

// Do runs fn and reports whether it panicked.
//
// A caught panic is reported before Do returns, so a caller has nothing to do
// with it but decide what the run's status is and whether there is anything
// left to run.
func (g Guard) Do(fn func()) (caught bool) {
	defer func() {
		v := recover()
		if v == nil {
			return
		}
		caught = true
		// Taken here rather than passed in: inside the deferred call the
		// panicking frames are still on the stack, so this is the trace of
		// where it happened and not of where it was caught.
		g.report(v, debug.Stack())
	}()
	fn()
	return false
}

// report writes what was caught.
//
// Plainly, and not through a dialect's Diagnostics. A dialect words what a
// shell says about a *script*; no shell has an opinion about this one, because
// no shell has this failure to describe, and there is no line of the script to
// locate it at — the position the panic came from is in the trace.
func (g Guard) report(v any, stack []byte) {
	if g.Err == nil {
		return
	}
	name := g.Name
	if name == "" {
		name = "sh"
	}
	// The write errors are discarded for the reason every other diagnostic
	// discards them: this is already the failure path, and there is nowhere
	// better to report a failure to report.
	_, _ = fmt.Fprintf(g.Err, "%s: internal error: %v\n", name, v)
	_, _ = fmt.Fprintf(g.Err,
		"%s: this is a bug in the shell rather than in the script; please report it at %s\n",
		name, tracker)
	if !g.Trace {
		_, _ = fmt.Fprintf(g.Err, "%s: set %s=1 for the stack trace\n", name, TraceVar)
		return
	}
	_, _ = g.Err.Write(stack)
}
