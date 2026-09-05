// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package policy is a declarative interp.Gate: a set of rules over paths and
// action kinds, and a default for everything they do not mention.
//
// It is the shipped answer to the seam docs/design.md opens. The seam is the
// interface; this is one implementation of it, and an embedder that wants
// something else — a prompt, an OS backend, a remote decision — writes its own
// Gate and never imports this package.
//
// docs/design/sandboxing.md is the specification: the file format, why the
// default posture is deny for every kind, why precedence is deny-overrides,
// and what a refusal looks like. What follows is the part a reader of the code
// needs.
//
// # The boundary this can and cannot draw
//
// It refuses what the *shell* does. A denied open stops the shell reading a
// file; an allowed exec starts a process that makes its own system calls, and
// nothing here has any say over those. `allow exec /bin/cat` is `allow read
// /**` spelled less obviously, and allowing an interpreter allows everything.
// Containing a running child needs the operating system, which sits above this
// layer. What this can do — and what nothing outside the interpreter can do,
// because `eval` walks around anything applied from outside — is refuse the
// shell's own accesses.
//
// # Dialect-blindness
//
// This package imports interp and the standard library. It imports nothing
// under dialect/, names no shell, and could not consult a semantics vector if
// it wanted to: Gate.Allow is handed a context and an interp.Action, and an
// Action carries a kind, a path, an argument vector, a write flag, a PID and a
// signal. There is no route from there to which shell is running, and a test
// asserts the import graph so that stays true.
//
// # Concurrency
//
// A Policy is immutable once built, so it needs no lock despite being consulted
// from every goroutine a shell has. Nothing here writes after Parse returns.
package policy

import (
	"context"
	"path/filepath"

	"github.com/blairham/sh/interp"
)

// A slot is what a rule and a default are really keyed by.
//
// Not the action kind, because one kind is two questions: an open for reading
// and an open for writing are the difference between a policy that lets a
// script read its input and one that lets it destroy its output, and a default
// keyed by ActionOpen could not tell them apart.
type slot uint8

const (
	slotExec slot = iota
	slotOpenRead
	slotOpenWrite
	slotStat
	slotReadDir
	slotSignal
	numSlots
)

// slotNone is the answer for an action no rule can be written about. It is
// deliberately outside the array: ActionInherit is recorded and never gated —
// see interp/seams.go — so nothing should ever reach here with one, and if
// something does it gets the policy's base default rather than an index panic.
const slotNone = slot(255)

func slotOf(a interp.Action) slot {
	switch a.Kind {
	case interp.ActionExec:
		return slotExec
	case interp.ActionOpen:
		if a.Write {
			return slotOpenWrite
		}
		return slotOpenRead
	case interp.ActionStat:
		return slotStat
	case interp.ActionReadDir:
		return slotReadDir
	case interp.ActionSignal:
		return slotSignal
	case interp.ActionInherit:
		return slotNone
	}
	return slotNone
}

// Selector is what a rule names in the file: a set of slots under one word.
//
// The vocabulary underneath is interp's and stays interp's, which is the rule
// internal/boundary follows for the same reason. A selector is a convenient
// name for a subset of it and never a new kind of action.
type Selector uint8

// The selectors, and what each covers. SelRead groups the probes with the read
// because reading a directory and asking whether a file is there are both
// reading: someone who writes `allow read /srv/**` and then finds `[ -f /srv/x ]`
// false has been given a policy that does not mean what it says.
const (
	SelExec Selector = iota
	SelRead
	SelWrite
	SelOpen
	SelStat
	SelList
	SelPath
	SelSignal
)

var selectorNames = map[string]Selector{
	"exec":   SelExec,
	"read":   SelRead,
	"write":  SelWrite,
	"open":   SelOpen,
	"stat":   SelStat,
	"list":   SelList,
	"path":   SelPath,
	"signal": SelSignal,
}

func (s Selector) String() string {
	for name, sel := range selectorNames {
		if sel == s {
			return name
		}
	}
	return "?"
}

// slots is the whole definition of what a selector selects, in one place, so
// that "does this rule match" and "which defaults does this set" cannot
// disagree about the answer.
func (s Selector) slots() []slot {
	switch s {
	case SelExec:
		return []slot{slotExec}
	case SelRead:
		return []slot{slotOpenRead, slotStat, slotReadDir}
	case SelWrite:
		return []slot{slotOpenWrite}
	case SelOpen:
		return []slot{slotOpenRead, slotOpenWrite}
	case SelStat:
		return []slot{slotStat}
	case SelList:
		return []slot{slotReadDir}
	case SelPath:
		// Every kind that carries a path, and deliberately not every kind: a
		// signal names a process rather than a file, so no pattern could
		// select one and `signal` has to be named on its own.
		return []slot{slotExec, slotOpenRead, slotOpenWrite, slotStat, slotReadDir}
	case SelSignal:
		return []slot{slotSignal}
	}
	return nil
}

// takesPattern reports whether a rule with this selector needs a path pattern.
// Only signals do not, because only signals have no path.
func (s Selector) takesPattern() bool { return s != SelSignal }

func (s Selector) covers(sl slot) bool {
	for _, have := range s.slots() {
		if have == sl {
			return true
		}
	}
	return false
}

// Rule is one line of a policy: a decision, the actions it speaks about, and
// the paths it speaks about. Pattern is empty exactly when Sel is SelSignal.
type Rule struct {
	Decision interp.Decision
	Sel      Selector
	Pattern  string
	// Alias is the other name for the same place, where the pattern names one
	// the platform has two names for, and empty otherwise. A rule matches
	// either, so this is always a widening and never a rewrite — see alias.go
	// for why keeping both is the answer here and not what a kernel does.
	//
	// It is filled in when the rule is parsed, so it costs nothing per decision
	// and reads nothing from the filesystem at all.
	Alias string
}

// String renders a rule the way a policy file writes one, with what it
// normalized to said out loud.
//
// Silence is the thing to avoid here. A rule that quietly covers a second path
// is a rule whose meaning is not in the file, and this is what lets a front end
// show the operator that the two names are one place — see cmd/sh, which prints
// it under -trace-events.
func (r Rule) String() string {
	out := decisionName(r.Decision) + " " + r.Sel.String()
	if r.Pattern != "" {
		out += " " + r.Pattern
	}
	if r.Alias != "" {
		out += " (also " + r.Alias + ")"
	}
	return out
}

// Policy is a rule set and its defaults. Build one with Parse or New; the zero
// value denies everything, which is the right thing for a value that was never
// filled in.
//
// The defaults are held as bools rather than as interp.Decision on purpose.
// interp.Allow is the zero value of Decision — correct there, since a Runner
// with no gate is unsandboxed — and a Policy whose zero value allowed
// everything would be a fail-open security type.
type Policy struct {
	rules []Rule
	// allowSlot[i] is the default for slot i, and setSlot[i] records that a
	// `default <decision> <selector>` line named it. Anything unnamed falls to
	// allowBase.
	allowSlot [numSlots]bool
	setSlot   [numSlots]bool
	allowBase bool
	// baseSet records that a bare `default` line was seen, so a second one
	// is an error rather than a silent override.
	baseSet bool
}

// New builds a policy in Go, for an embedder that has no file.
//
// def is the default for every slot. Rules are as they would be written in a
// file, and mean the same thing: deny overrides, order does not matter.
func New(def interp.Decision, rules ...Rule) *Policy {
	p := &Policy{rules: rules, allowBase: def == interp.Allow, baseSet: true}
	for i := range p.allowSlot {
		p.allowSlot[i] = p.allowBase
	}
	return p
}

// Rules returns the rule set, for a caller that wants to show a policy back.
// A copy, because a Policy is immutable and handing out the slice would stop
// being true the moment somebody wrote to it.
func (p *Policy) Rules() []Rule {
	if p == nil {
		return nil
	}
	return append([]Rule(nil), p.rules...)
}

// Allow decides one action. It is interp.Gate.
//
// The evaluation is deny-overrides and order-independent: any matching deny
// refuses, otherwise any matching allow permits, otherwise the default for the
// action's slot. docs/design/sandboxing.md gives the argument, and it is
// composition — concatenating two policies has to allow only what both allow,
// and no order-dependent precedence has that property.
func (p *Policy) Allow(_ context.Context, a interp.Action) interp.Decision {
	if p == nil {
		// A nil *Policy in a non-nil interp.Gate is a wiring mistake, and the
		// fail-closed answer is the only safe one: a security type that
		// allowed everything when it had not been filled in would be worse
		// than no gate at all, because something would report being gated.
		return interp.Deny
	}
	sl := slotOf(a)
	path, addressable := matchPath(a.Path)
	allowed := false
	for _, r := range p.rules {
		if !r.Sel.covers(sl) {
			continue
		}
		if r.Sel.takesPattern() {
			if !addressable || !r.matches(path) {
				continue
			}
		}
		if r.Decision == interp.Deny {
			return interp.Deny
		}
		allowed = true
	}
	if allowed || p.defaultFor(sl) {
		return interp.Allow
	}
	// An `allow exec` rule also permits the stat that finds the program, and
	// only where nothing else has decided the question — an explicit deny
	// still wins, above. A policy that lets a script run a program but not
	// discover it does not mean what it says: a PATH search stats each
	// candidate, and `command -v` and `[ -x ]` ask the same question the
	// search does.
	//
	// It leaks nothing the policy had not already given away. The stat can
	// only succeed for a path the script was going to be allowed to execute.
	if sl == slotStat && addressable && p.execAllows(path) {
		return interp.Allow
	}
	return interp.Deny
}

func (p *Policy) defaultFor(sl slot) bool {
	if sl == slotNone || sl >= numSlots {
		return p.allowBase
	}
	return p.allowSlot[sl]
}

// matches reports whether a path is one this rule speaks about, under either of
// the names the platform has for it.
func (r Rule) matches(path string) bool {
	return match(r.Pattern, path) || (r.Alias != "" && match(r.Alias, path))
}

// Normalized is the rules that gained a second name when they were parsed, for
// a caller that wants to tell an operator what their file turned into.
func (p *Policy) Normalized() []Rule {
	if p == nil {
		return nil
	}
	var out []Rule
	for _, r := range p.rules {
		if r.Alias != "" {
			out = append(out, r)
		}
	}
	return out
}

func (p *Policy) execAllows(path string) bool {
	for _, r := range p.rules {
		if r.Decision == interp.Allow && r.Sel == SelExec && r.matches(path) {
			return true
		}
	}
	return false
}

// matchPath prepares an action's path for matching, and reports whether any
// pattern could match it at all.
//
// Cleaned lexically, so `/srv/../etc/x` is matched as `/etc/x` and a `..`
// cannot walk out of a pattern.
//
// A relative path is *not* addressable, and that is the fail-closed reading
// rather than an oversight. Patterns are absolute because a relative one would
// mean "relative to a working directory" and the policy owns none — the
// shell's directory moves with `cd` and is not the process's. The interpreter
// resolves against the Runner's directory before it asks, so a relative path
// arriving here means something upstream did not, and the default (deny, in
// any policy worth the name) is the right answer for it.
func matchPath(p string) (string, bool) {
	if p == "" || !filepath.IsAbs(p) {
		return "", false
	}
	return filepath.Clean(p), true
}
