// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"io"
	"os"

	"github.com/blairham/sh/internal/panicguard"
)

// guard is what this shell catches an interpreter bug with.
//
// Every route a shell can be invoked by ends up running one, because a panic
// costs whatever the guard around it was holding: the whole of a script on the
// `-c` and file routes, which are not sessions and have nothing to keep going
// for; one startup file where a session is being built; and one typed line at
// a prompt, which repl holds because it is the only thing that knows where a
// line ends. interp itself keeps panicking, which is correct for a library —
// see internal/panicguard for the whole of the reasoning.
func (sh Shell) guard() panicguard.Guard {
	return panicguard.Guard{Name: sh.Name, Err: sh.Stderr, Trace: panicTrace()}
}

// guardConcurrent is the same guard, for the goroutines the shell runs beside
// itself — a background job, either half of a pipeline, a coprocess, a process
// substitution.
//
// It has to be a second one because `recover` reaches only the goroutine that
// panicked: the guard above is on the goroutine the run was started from, and
// every one of those four is out of its reach. So interp calls this one on the
// goroutine it just started, which is the only place a recover for it can be —
// the same split as everywhere else, with interp declining to make the decision
// a library may not make and the front end making it.
//
// The stream is interp's rather than sh.Stderr, and taking it rather than
// reaching for the one this Shell was built with is the point. interp
// serializes writes to a caller's io.Writer with a lock of its own, because it
// is what created the concurrency; a report written straight to sh.Stderr would
// be a second writer with no lock at all, racing the shell's own diagnostics on
// the writer they share. What comes through the seam is the stream with that
// lock already on it.
//
// There is no status to return. Nothing is waiting on an answer here — a
// background job's status is the job's, and interp records it either way — so
// what is left is the diagnostic, which is the whole of what a person gets to
// see that anything happened at all.
func (sh Shell) guardConcurrent() func(func(), io.Writer) {
	name, trace := sh.Name, panicTrace()
	return func(work func(), errs io.Writer) {
		_ = panicguard.Guard{Name: name, Err: errs, Trace: trace}.Do(work)
	}
}

// panicTrace reports whether a caught panic should print its stack.
//
// Read here rather than in the guard, for the reason every other read of
// process state is made in this package: driver is the front end and *is* the
// process, while interp and repl are libraries an embedder links, where
// reaching for the environment is the leak the purity rule exists to stop. The
// answer travels down as a value, the same way the environment itself does.
func panicTrace() bool { return os.Getenv(panicguard.TraceVar) != "" }
