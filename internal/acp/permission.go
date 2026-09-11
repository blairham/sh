// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acp

import (
	"context"
	"sync"

	"github.com/blairham/sh/interp"
)

// The permission model, which belongs to neither side of the protocol.
//
// ACP has two roles and this shell is both: an editor drives it as an Agent,
// and it drives a coding agent as a Client. Turning a permission option into a
// decision is the same function in both — the four kinds mean what they mean
// whoever is looking at them, and two of them allow, two refuse, and one of
// each is remembered. What differs is only who is asked, which is why that is
// a func field here and not a branch.
//
// docs/design/acp.md has the reasoning; this file is the whole of the
// mechanism.

// Decision is what an answer came to: whether the action may proceed, and
// whether the answer should be kept for the next identical one.
type Decision struct {
	Allow    interp.Decision
	Remember bool
}

// Decide reads a permission outcome.
//
// Everything that is not an explicit allow is a denial, and the list of ways
// that happens is the point: a turn canceled before the person answered, a
// reject option, and — the one worth naming — an option id that was never
// offered. A client answering with something we did not put in the list is a
// client we cannot interpret, and guessing at it would be inventing consent.
func Decide(out PermissionOutcome) Decision {
	if out.Outcome != OutcomeSelected {
		return Decision{Allow: interp.Deny}
	}
	switch out.OptionID {
	case OptionAllowOnce:
		return Decision{Allow: interp.Allow}
	case OptionAllowAlways:
		return Decision{Allow: interp.Allow, Remember: true}
	case OptionRejectAlways:
		return Decision{Allow: interp.Deny, Remember: true}
	}
	// OptionRejectOnce and anything unrecognized.
	return Decision{Allow: interp.Deny}
}

// Escalates reports whether an action is worth stopping a person for.
//
// Not every action, and this is a judgement rather than a reading of the
// protocol. One `ls | grep x` stats every PATH entry it tries and a glob stats
// and reads directories in bulk; a permission prompt arriving at that rate is
// one people click through, which is a *worse* boundary than an honest record
// because it turns a considered answer into a reflex.
//
// So: the actions that change something outside the shell, or start something
// the boundary can no longer see. An exec, because a command is the thing a
// person means to approve and its own accesses are its own once it is running.
// An open for writing, because that is the shell changing the file system in
// its own right. A signal, because reaching another process is the same shape
// of act as starting one.
//
// Reads — a non-writing open, a stat, a directory read — are allowed and
// recorded, never asked about. Refusing those by rule is a policy language and
// belongs to docs/design/sandboxing.md, which this composes with rather than
// duplicates. ActionInherit is never here because the interpreter never asks
// the gate about it at all.
//
// A write to a discarding device is the reads' case rather than the writes':
// see discardingDevices.
func Escalates(a interp.Action) bool {
	switch a.Kind {
	case interp.ActionExec, interp.ActionSignal:
		return true
	case interp.ActionOpen:
		return a.Write && !discards(a)
	case interp.ActionStat, interp.ActionReadDir, interp.ActionInherit:
		return false
	}
	return false
}

// discardingDevices are the device files a write to leaves nothing behind.
//
// The paragraph above is the whole argument for this, applied to the one
// write that is in almost every script: `>/dev/null` appears several times a
// line in ordinary shell, and approving it protects nothing — the bytes go
// nowhere, nothing afterwards can observe that the write happened, and a
// client that refuses it changes only whether the command works. A prompt
// arriving at that rate is the reflex-making prompt Escalates exists to
// avoid, and it arrived on the *first* script the protocol instrument ran
// (#1813).
//
// A fixed set of names rather than a rule about paths, and the difference is
// the point. "Anything under /dev" or a prefix list would be the sandboxing
// policy language written badly in the wrong package, which is exactly what
// the comment above rules out; `/dev/sda` and `/dev/tty` are writes that
// change the world as much as any file does. These three are a different kind
// of statement — named devices whose contract *is* that a write to them is
// unobservable — and the set does not grow without the same argument being
// made again about a specific name.
//
// `/dev/tty` is deliberately absent. A write there is seen by a person, which
// is a thing that happens outside the shell; the argument for these three is
// that nothing happens at all.
var discardingDevices = map[string]bool{
	"/dev/null": true,
	"/dev/zero": true,
	"/dev/full": true,
}

// discards reports whether an open reached one of those devices.
//
// Both names are asked, because either can be the device: Path is what the
// script wrote and Resolved is the kernel's own name where the two differ, so
// a link to /dev/null is still a write to /dev/null. Exact spellings only — a
// path this has to normalize first is a path rule, and a path rule is the
// policy language this is not.
func discards(a interp.Action) bool {
	return discardingDevices[a.Path] || discardingDevices[a.Resolved]
}

// Memory holds the answers a person asked to have kept.
//
// Keyed on what was asked about rather than on the words of the question: the
// kind of action and the path, plus whether an open was for writing, so that
// "allow always" for reading a file does not quietly allow writing it. A
// signal's key is its target pid, which is deliberately near-useless — a pid
// is not a stable identity, and an answer remembered about one is worth about
// as much as the pid is.
//
// Exact, and never a prefix. "Allow always for everything under /tmp" is a
// policy language; writing one here would be the sandboxing work done badly in
// the wrong package.
//
// Guarded, because a gate is consulted from every goroutine a shell has: a
// background job and each half of a pipeline ask on their own.
type Memory struct {
	mu sync.Mutex
	m  map[memoryKey]interp.Decision
}

type memoryKey struct {
	kind  interp.ActionKind
	path  string
	write bool
	pid   int
}

func keyOf(a interp.Action) memoryKey {
	k := memoryKey{kind: a.Kind, path: a.Path, write: a.Write}
	if a.Kind == interp.ActionSignal {
		k.pid = a.PID
	}
	return k
}

// Recall reports the remembered answer for an action, if there is one.
func (m *Memory) Recall(a interp.Action) (interp.Decision, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.m[keyOf(a)]
	return d, ok
}

// Remember keeps an answer for the next identical action.
func (m *Memory) Remember(a interp.Action, d interp.Decision) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[memoryKey]interp.Decision{}
	}
	m.m[keyOf(a)] = d
}

// Asker turns an action into a question somebody answers.
//
// This is the one role-specific thing in the file, and it is a function rather
// than an interface with two implementations because there is nothing to
// implement: as an Agent it sends session/request_permission and reads the
// reply; as a Client it puts the question in front of whoever is at the shell.
//
// An error is a denial. A question that could not be asked has not been
// answered, and there is no other honest reading of that.
type Asker func(ctx context.Context, a interp.Action) (Decision, error)

// Gate is the permission gate both roles install on a shell.
//
// It is an interp.Gate, so the interpreter's question stays the binary one it
// has always been. Who answers it is arranged here.
type Gate struct {
	// Inner is the policy, and nil means no policy. It is consulted first
	// and its refusal is final: a person offered a button that overrides the
	// sandbox is a sandbox that is advisory, which is not a sandbox. This is
	// also why interp needs no third `Ask` decision — the escalation happens
	// between two gates rather than inside one.
	Inner interp.Gate

	// Ask is who to ask. Nil denies every action that would have been asked
	// about, which is the safe reading of "there is nobody there": a shell
	// with an escalating gate and no way to escalate must not proceed
	// quietly.
	Ask Asker

	// Escalate decides which actions are worth asking about. Nil is
	// Escalates, which is the default the design document argues for.
	Escalate func(interp.Action) bool

	// Remembered is where `allow always` and `reject always` are kept. Nil
	// remembers nothing, so every question is asked again — correct, if
	// tiring.
	Remembered *Memory
}

// Allow answers the interpreter.
func (g *Gate) Allow(ctx context.Context, a interp.Action) interp.Decision {
	if g.Inner != nil && g.Inner.Allow(ctx, a) == interp.Deny {
		// Refused by policy. Nobody is asked, and nothing is remembered:
		// there is no answer here to keep, only a rule that already applied.
		return interp.Deny
	}
	escalate := g.Escalate
	if escalate == nil {
		escalate = Escalates
	}
	if !escalate(a) {
		return interp.Allow
	}
	if g.Remembered != nil {
		if d, ok := g.Remembered.Recall(a); ok {
			return d
		}
	}
	if g.Ask == nil {
		return interp.Deny
	}
	d, err := g.Ask(ctx, a)
	if err != nil {
		return interp.Deny
	}
	if d.Remember && g.Remembered != nil {
		g.Remembered.Remember(a, d.Allow)
	}
	return d.Allow
}
