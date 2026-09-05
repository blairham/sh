// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/interp"
)

// The debug route onto the gate and the event stream.
//
// The seam is interpreter-internal and was unit-tested from the first commit,
// but until this file nothing that ships ever set either one: no binary
// mentioned a Gate or a Sink, so the conformance harness and the wild sweep
// both ran ungated, and a hole in the boundary would have looked exactly like
// a shell that works. One reachable consumer is what turns the seam from a
// claim into something a person can point a script at and watch.
//
// It is a debug surface rather than a sandbox, and the distinction is worth
// stating so nobody mistakes it for one. The boundary is drawn around the
// interpreter and not around the process tree: `-deny /secret` hides /secret
// from the shell's own file tests, globs and redirections, and does nothing
// to a `cat /secret/f` the shell was allowed to start, because a child makes
// its own accesses. Containing a command once it is running wants an OS
// sandbox backend, which sits above the substrate — see docs/design.md.

// installSeams fills in the shell's Gate and Sink from what the invocation
// asked for, and leaves both nil when it asked for neither.
//
// Nil is not an oversight here, it is the contract: a nil Gate allows
// everything and a nil Sink discards, so a shell nobody has pointed a policy
// at costs one nil check per action and behaves exactly as it did before
// either flag existed.
//
// Assembled in a function of its own so a test can look at what the flags
// produced. Wiring dropped on the floor inside main() is invisible — it looks
// precisely like a shell that was never asked to gate anything, which is the
// failure mode this whole change is about.
func installSeams(sh driver.Shell, own ownFlags, w io.Writer) driver.Shell {
	if len(own.deny) > 0 {
		sh.Gate = denyPrefixes(own.deny)
	}
	if own.traceEvents {
		sh.Events = &traceSink{w: w}
	}
	return sh
}

// traceSink prints every event to a writer, one line each.
//
// The mutex is the contract and not caution. A Sink is called from more than
// one goroutine — a background job reports from the goroutine running it, and
// so does each half of a pipeline — so an unguarded writer here would
// interleave two events into one unreadable line, and race detection would
// call it what it is.
type traceSink struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *traceSink) Emit(_ context.Context, e interp.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A trace that cannot be written is dropped. Complaining about it would
	// mean a diagnostic per action on a closed stream, and there is nowhere
	// to complain to that is not the stream that just failed.
	_, _ = fmt.Fprintln(s.w, formatEvent(e))
}

// formatEvent renders one event as a line of key-value text.
//
// The event is structured because its consumers are not people; this is the
// one consumer that is, so the formatting lives here rather than in interp.
// Everything an event carries appears, and nothing that is absent is printed:
// a status only means something at the end of a command, an error only when
// something failed, and a file only when the line came from one.
func formatEvent(e interp.Event) string {
	var b strings.Builder
	fmt.Fprintf(&b, "trace: %s %s %s", e.Kind, e.Action.Kind, e.Action.Path)
	if e.Action.Args != nil {
		fmt.Fprintf(&b, " args=%q", e.Action.Args)
	}
	if e.Action.Write {
		b.WriteString(" write=true")
	}
	if e.Kind == interp.EventCommandEnd {
		fmt.Fprintf(&b, " status=%d", e.Status)
	}
	if e.Err != nil {
		fmt.Fprintf(&b, " err=%q", e.Err.Error())
	}
	fmt.Fprintf(&b, " line=%d", e.Line)
	if e.File != "" {
		fmt.Fprintf(&b, " file=%s", e.File)
	}
	return b.String()
}

// denyPrefixes refuses any action on a path at or beneath one of them.
//
// Whole path components, so `-deny /etc` refuses /etc and /etc/passwd and
// leaves /etcetera alone. A prefix that matched by characters would refuse a
// neighboring directory because its name starts the same way, which is not
// what anyone typing a directory means.
//
// Every kind of action, deliberately. What a refusal then *looks like* is the
// interpreter's and differs by kind — a denied exec says so and fails, a
// denied stat answers as a missing path does and says nothing — and that
// difference is the point of pointing this at a real script.
//
// It keeps no state, so it needs no lock despite being called from several
// goroutines at once.
type denyPrefixes []string

func (d denyPrefixes) Allow(_ context.Context, a interp.Action) interp.Decision {
	for _, p := range d {
		if p == "" {
			continue
		}
		if a.Path == p || strings.HasPrefix(a.Path, strings.TrimSuffix(p, "/")+"/") {
			return interp.Deny
		}
	}
	return interp.Allow
}
