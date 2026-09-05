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
	"github.com/blairham/sh/internal/policy"
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
		d, err := denyRules(own.deny)
		if err != nil {
			return sh, nil, err
		}
		g = append(g, d)
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

// denyRules turns the -deny values into a gate, and it is the *policy engine's*
// rule language rather than a second one.
//
// This used to be a path-prefix list of its own, and that was two matchers over
// one gate. Two matchers is two answers to the same question and the one nobody
// exercises is the one that is wrong — this one was: it compared a path by
// lexical prefix where internal/policy cleans a path before matching it, so
// `-deny /srv` did not cover `/srv/../srv/x` and `deny path /srv/**` did. It
// also had a hole it could not close: every rule it could express named a path,
// and a signal names a process, so `-trace-events` could watch a signal and
// nothing shipped could refuse one. The policy language has no such hole,
// because its vocabulary is interp's action kinds rather than paths.
//
// So a `-deny` value is a policy rule minus its decision word, and the whole of
// the difference is the separator:
//
//	-deny /etc              # every kind that has a path, at or under /etc
//	-deny path:/etc/**      # the same rule, written out
//	-deny signal            # every signal this shell sends
//	-deny exec:/usr/bin/**  # one kind, one subtree
//
// A colon rather than a space, and that is the one concession to being a flag:
// a value is one shell word, so `-deny exec /usr/bin` would hand `exec` to the
// flag and `/usr/bin` to the shell as a script — which runs, quietly, under a
// policy the person did not write. Every form above is a single word and needs
// no quoting, which is the property worth having here. The selectors, the
// patterns and the matcher are the file's, unchanged.
//
// The bare path is the shorthand the flag has always had, and it now means what
// it always said it meant: `-deny /etc` is `deny path /etc/**`, and `**`
// matches zero or more components, so it covers /etc itself and everything
// beneath. Globs work in it, which they did not before — that follows from
// there being one pattern language rather than two.
func denyRules(values []string) (interp.Gate, error) {
	rules := make([]policy.Rule, 0, len(values))
	for _, v := range values {
		r, err := policy.ParseRule(interp.Deny, denyBody(v))
		if err != nil {
			return nil, fmt.Errorf("-deny %s: %w", v, err)
		}
		rules = append(rules, r)
	}
	// Allow-everything with holes cut in it, which is what a debug surface is:
	// a way to watch the gate refuse something rather than a sandbox. A policy
	// file is how a sandbox is written, and composing the two intersects them —
	// see gates in sandbox.go.
	return policy.New(interp.Allow, rules...), nil
}

// denyBody turns a flag value into the body of a policy rule.
//
// Three shapes, distinguished without ambiguity: a value beginning with `/` is
// a path, because a selector is a bare word and no selector begins with a
// slash; otherwise the first colon separates the selector from the pattern, and
// a colon is legal in a path so only the first one splits. A value with no
// colon is a selector on its own, which is what `signal` needs.
func denyBody(v string) string {
	if strings.HasPrefix(v, "/") {
		// Trailing slashes trimmed so `-deny /etc/` and `-deny /etc` are the
		// same rule. Without it the pattern would be `/etc//**`, which names a
		// directory nobody has.
		return "path " + strings.TrimSuffix(v, "/") + "/**"
	}
	sel, pattern, ok := strings.Cut(v, ":")
	if !ok {
		return v
	}
	return sel + " " + pattern
}
