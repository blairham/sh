// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package interp executes a syntax tree.
//
// Two seams are built in from the first commit rather than added later, and
// docs/design.md gives the reason: a policy enforced *outside* the interpreter
// does not reach inside it. `eval`, `source`, command substitution and
// subshells all re-enter with their own input, so anything that must hold
// universally — a sandbox boundary, an audit trail, a permission prompt — has
// to live at the point of execution or the first `eval` walks around it.
//
// Sandboxing, agent permission prompts and AI context turn out to be the same
// two questions: may this proceed, and what just happened. So there are two
// primitives rather than three subsystems.
package interp

import "context"

// ActionKind is what an action does. These are the points where control
// leaves the interpreter's own memory, which is the complete boundary worth
// gating: everything else is arithmetic on values it already holds.
type ActionKind uint8

const (
	// ActionExec runs a program.
	ActionExec ActionKind = iota
	// ActionOpen opens a file for a redirection.
	ActionOpen
)

func (k ActionKind) String() string {
	if k == ActionOpen {
		return "open"
	}
	return "exec"
}

// Action describes something about to happen.
type Action struct {
	Kind ActionKind
	// Path is the program or file.
	Path string
	// Args is the argument vector for an exec, nil otherwise.
	Args []string
	// Write is set when an open is for writing.
	Write bool
}

// Decision is a gate's answer.
type Decision uint8

const (
	// Allow lets the action proceed.
	Allow Decision = iota
	// Deny refuses it. The interpreter reports the refusal and carries on
	// with a failing status rather than aborting, because a denied command
	// is a command that failed, not a broken shell.
	Deny
)

// Gate decides whether an action may proceed.
//
// A nil Gate allows everything, so the zero value of a Runner is usable and
// nothing has to opt in to being unsandboxed.
//
// Called from more than one goroutine. A background job runs on its own, and
// so does each half of a pipeline, so a gate that keeps anything — a count, a
// log, a set of paths already allowed — has to guard it. The first gate
// written against this package was a closure appending to a slice, and it had
// a data race the moment a script said `&`.
type Gate interface {
	Allow(ctx context.Context, a Action) Decision
}

// GateFunc adapts a function to Gate.
type GateFunc func(context.Context, Action) Decision

func (f GateFunc) Allow(ctx context.Context, a Action) Decision { return f(ctx, a) }

// EventKind classifies an event.
type EventKind uint8

const (
	// EventCommandStart is emitted before a command runs, after the gate
	// allowed it.
	EventCommandStart EventKind = iota
	// EventCommandEnd carries the exit status.
	EventCommandEnd
	// EventDenied is emitted when a gate refused.
	EventDenied
	// EventError carries a failure that was not an exit status — a program
	// that could not be found, a file that could not be opened.
	EventError
)

func (k EventKind) String() string {
	switch k {
	case EventCommandEnd:
		return "command-end"
	case EventDenied:
		return "denied"
	case EventError:
		return "error"
	}
	return "command-start"
}

// Event is a structured record of something that happened.
//
// It is structured rather than a formatted line because its consumers are not
// people: an agent protocol streams it, an AI assistant assembles context from
// it, an audit trail stores it. Formatting is the caller's business.
type Event struct {
	Kind   EventKind
	Action Action
	// Status is the exit status, meaningful for EventCommandEnd.
	Status int
	// Err is the failure, for EventError.
	Err error
}

// Sink receives events. A nil Sink discards them.
//
// Called from more than one goroutine, for the same reason a Gate is: what a
// background job does is reported from the goroutine running it.
type Sink interface {
	Emit(ctx context.Context, e Event)
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(context.Context, Event)

func (f SinkFunc) Emit(ctx context.Context, e Event) { f(ctx, e) }
