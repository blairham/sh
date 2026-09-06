// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"io"
	"os"
	"sync"
)

// The streams this invocation was handed, guarded for the one thing in this
// binary that writes them from a goroutine of its own.
//
// A plugin host relays the plugin's standard error onto the shell's, from a
// goroutine that lives as long as the plugin does — see internal/plugin's
// relay. The shell writes the same stream at the same time, and when an
// external command's stderr is not an *os.File, os/exec copies it there on a
// goroutine of its own as well. Two writers, one io.Writer, no shared lock:
// the race detector caught exactly that pair on Linux (#1138), a `fmt.Fprintf`
// from the relay against an `io.Copy` from `os/exec`.
//
// This is interp's lockedWriter rule met from outside the interpreter, and the
// rule is the one that matters: **the shell creates the concurrency, so the
// shell owns the synchronization** — requiring callers to pass thread-safe
// writers would be a surprising thing to demand of an interface that says
// io.Writer. interp guards the streams it hands a pipeline, a background job
// and a process substitution, and it cannot guard this one, because interp
// does not know a plugin host exists. This binary is the only place that hands
// one writer to both, so it is the only place the lock can be put.
//
// It is not #1048's shape and no amount of waiting fixes it. There the end of
// a *call* had to be waited for, because a host method arriving behind an
// invoke response reached a Runner that had moved on; the fix was to make the
// close wait. Here the two writes are legitimately simultaneous — a plugin
// complaining while a command is running is the case the relay exists for —
// and the relay is already joined before run returns, by Host.Close. There is
// no ordering to restore, only an overlap to exclude.

// guardedStreams returns stdout and stderr behind one mutex.
//
// One mutex for both, not one each, for the reason interp's streamLocks has
// one: `Stdout` and `Stderr` are two fields and need not be two writers, and
// an embedder that wants what `2>&1` gives hands the same buffer to both. A
// lock per field would then be two locks over one object, which excludes
// nothing.
//
// An *os.File is left alone, which is the same rule and the same two reasons:
// two goroutines writing one file are serialized by the descriptor's own lock,
// and wrapping one would make os/exec build a pipe for every child instead of
// handing the file over — so a child asking whether its output is a terminal
// would get a different answer for the rest of the session. In the shipped
// binary both streams *are* files, so this changes nothing a person running
// `sh` can observe; what it covers is an embedder's writer, which is where the
// concurrency has nowhere else to be serialized.
func guardedStreams(stdout, stderr io.Writer) (io.Writer, io.Writer) {
	mu := new(sync.Mutex)
	return guardWriter(mu, stdout), guardWriter(mu, stderr)
}

func guardWriter(mu *sync.Mutex, w io.Writer) io.Writer {
	if _, isFile := w.(*os.File); isFile {
		return w
	}
	return &guardedWriter{mu: mu, w: w}
}

type guardedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (g *guardedWriter) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.w.Write(p)
}
