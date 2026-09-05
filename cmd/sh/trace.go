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
	"github.com/blairham/sh/internal/event"
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
// at costs one nil check per action and behaves exactly as it did before any
// of these flags existed.
//
// Assembled in a function of its own so a test can look at what the flags
// produced. Wiring dropped on the floor inside main() is invisible — it looks
// precisely like a shell that was never asked to gate anything, which is the
// failure mode this whole change is about.
//
// The closer is the audit file, when there is one, and it is returned rather
// than deferred here because main ends with os.Exit and a defer would never
// run. Nothing is lost when it is skipped — a record is written straight
// through — but a file left open by a process that is exiting anyway is
// untidy in exactly the way that later reads as a leak.
func installSeams(sh driver.Shell, own ownFlags, w io.Writer) (driver.Shell, io.Closer, error) {
	var g gates
	if len(own.deny) > 0 {
		g = append(g, denyPrefixes(own.deny))
	}
	if own.policy != "" {
		p, err := loadPolicy(own.policy)
		if err != nil {
			return sh, nil, err
		}
		g = append(g, p)
	}
	var s sinks
	if own.traceEvents {
		s = append(s, &traceSink{w: w})
	}
	var closer io.Closer
	if own.audit != "" {
		aw, c, err := openAudit(own.audit, w)
		if err != nil {
			return sh, nil, err
		}
		closer = c
		s = append(s, event.NewEncoder(aw))
	}
	// One of a kind is installed as itself rather than as a list of one, so
	// the common case pays nothing for the composition and a stack trace names
	// what is actually deciding.
	switch len(g) {
	case 0:
	case 1:
		sh.Gate = g[0]
	default:
		sh.Gate = g
	}
	switch len(s) {
	case 0:
	case 1:
		sh.Events = s[0]
	default:
		sh.Events = s
	}
	return sh, closer, nil
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
	fmt.Fprintf(&b, "trace: %s %s", e.Kind, e.Action.Kind)
	if e.Action.Path != "" {
		fmt.Fprintf(&b, " %s", e.Action.Path)
	}
	if e.Action.Kind == interp.ActionSignal {
		// A signal names a process rather than a path, so it is the one kind
		// with nothing to print above. The number rather than a name: which
		// numbers exist is not the same on two operating systems, and the
		// table that answers that is the interpreter's.
		fmt.Fprintf(&b, " pid=%d signal=%d", e.Action.PID, int(e.Action.Signal))
	}
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
	if e.Action.ID != "" {
		// Last, and only when there is one. It is what pairs a start with its
		// end by eye, which ordering cannot do here: a background job and each
		// half of a pipeline write from their own goroutines, so the line after
		// a start is very often another command's.
		//
		// The session is deliberately not printed. A trace is one shell writing
		// to one stream, so it would be the same string on every line — the
		// audit record carries it because a file several shells append to needs
		// it, and this is not that.
		fmt.Fprintf(&b, " id=%s", e.Action.ID)
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
// Every kind of action that has a path, deliberately. What a refusal then
// *looks like* is the interpreter's and differs by kind — a denied exec says
// so and fails, a denied stat answers as a missing path does and says
// nothing — and that difference is the point of pointing this at a real
// script. A signal names a process instead, so nothing a path list holds can
// match one: `-trace-events` watches signals and `-deny` cannot refuse them,
// which is a limit of this debug surface rather than of the gate. Refusing by
// something other than a path is what a real policy is for.
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
