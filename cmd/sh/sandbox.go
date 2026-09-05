// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// -policy and -audit: the route that makes the gate a sandbox rather than
// something to watch.
//
// -deny and -trace-events came first and are a debug surface — a way to see a
// refusal happen. These are the shipped policy: a declarative rule set from a
// file, and the event stream written down in the schema its consumers share.
//
// The distinction that matters is unchanged and is worth repeating where
// somebody reads the flags. The boundary is drawn around the interpreter and
// not around the process tree: a policy refuses what the *shell* opens, stats
// and runs, and a command the shell was allowed to start makes its own
// accesses that nothing here sees. `allow exec /bin/cat` is `allow read /**`
// spelled less obviously. Containing a running child needs an OS sandbox,
// which sits above the substrate — see docs/design/sandboxing.md.
//
// A policy is never discovered. There is no environment variable and no
// dotfile searched for, because a policy nameable by the environment is
// replaceable by anything that can set it — including the sandboxed script, on
// its way to invoking a nested shell. It comes from this flag or from an
// embedder assigning driver.Shell.Gate, and from nowhere else.
//
// Neither the policy file nor the audit stream passes the gate. That is an
// exemption against the rule docs/design.md states — an access is inside the
// boundary when the path was chosen by whoever the policy is about — and the
// reason is subject versus apparatus: the script is what the policy is about,
// while these two are the policy's own machinery, and a boundary that could be
// told to stop reading its rules or stop recording what it did is not one.
// Both are also opened before there is a gate to ask.

// openAudit opens the destination for the event stream. A lone `-` is the
// stream the trace already uses, which is standard error.
//
// Appended rather than truncated, because an audit trail that erases the
// previous run on the next one is not an audit trail, and 0600 because a
// record of what a script reached for names paths that are nobody else's
// business.
func openAudit(name string, stderr io.Writer) (io.Writer, io.Closer, error) {
	if name == "-" {
		return stderr, nil, nil
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, nil, err
	}
	// Unbuffered on purpose: json.Encoder writes each record straight through,
	// so a shell that dies mid-script has still recorded everything up to the
	// action that killed it. A buffer would lose exactly the records an
	// investigation wants.
	return f, f, nil
}

// gates is every gate an invocation asked for, consulted as one.
//
// Any refusal refuses, which makes composition an intersection: two policies
// together allow only what both allow. That is the same rule the policy file
// uses between its own lines, and it is the only composition that lets someone
// add `-deny /tmp/x` to an existing policy and be sure they narrowed it.
type gates []interp.Gate

func (g gates) Allow(ctx context.Context, a interp.Action) interp.Decision {
	for _, one := range g {
		if one.Allow(ctx, a) == interp.Deny {
			return interp.Deny
		}
	}
	return interp.Allow
}

// sinks is every sink an invocation asked for, fed as one. A trace to watch by
// eye and an audit file to keep are different jobs and a person may want both.
type sinks []interp.Sink

func (s sinks) Emit(ctx context.Context, e interp.Event) {
	for _, one := range s {
		one.Emit(ctx, e)
	}
}

// loadPolicy reads the policy file named on the command line.
//
// The concrete type rather than the interface, because the caller has one more
// question for it than a Gate can answer: which of its rules gained a second
// name when they were read. A boundary that normalizes silently is a boundary
// whose meaning is not in the file it came from.
func loadPolicy(name string) (*policy.Policy, error) {
	p, err := policy.ParseFile(name)
	if err != nil {
		return nil, fmt.Errorf("policy: %w", err)
	}
	return p, nil
}
