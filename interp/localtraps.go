// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"sort"
	"syscall"
)

// Traps that are local to the function that set them.
//
// The seam is a moment and a store: the moment a `trap` command changes a
// condition, and a place on the running call to keep what it displaced. Which
// dialect asks for it, and on what grounds, is Semantics.FunctionLocalTraps —
// nothing here knows a shell's name or an option's.
//
// # Why the save is here and not at the call
//
// AtEveryFunctionCall gives a dialect the start of every body, which is the
// right moment for an option *table* — a copy of it is small, and the shell
// that scopes options restores one that moved before the line asking for the
// scoping. Traps are the other answer to the same-looking question, measured:
// a trap moved before that line is **not** restored, and a trap moved after
// the scoping is turned back off **is**. So the question is asked once per
// modification, at the modification, and a save taken at the entry would be
// answering a question nobody asked.
//
// That is also why the two options in the shell that has both are two
// mechanisms rather than one lever with two names, which the corpus pins from
// the other side as well: `localoptions` leaves a function's trap installed.
//
// # What a save holds
//
// The disposition, not the listing. A condition with no trap goes back to
// having none — measured, a signal sent after such a return kills the shell
// where the function's handler would have caught it — so "nothing was set" is
// a value here and not an absence, and an ignore (`trap '' INT`) is a third
// state that is neither.

// savedTrapState is one condition as it stood before a function displaced it.
type savedTrapState struct {
	// name is the canonical condition: a signal's name, or a
	// pseudo-condition. EXIT never reaches here — see localizeTrap.
	name string
	// sig is the signal to arrange for again, meaningless for a
	// pseudo-condition.
	sig syscall.Signal
	// action is what was set: nil for nothing at all, and a pointer to the
	// empty string for an ignore.
	action *string
	// pseudo marks a condition the interpreter fires itself, which lives in
	// a slot of its own rather than in the signal table.
	pseudo bool
	// frame is where a pseudo-trap had been set, which decides where it
	// fires; inherited says it arrived across a subshell boundary rather
	// than being set on this side of one. Both travel with the action,
	// because putting the text back without them would restore a trap that
	// fires in the wrong places.
	frame     int
	inherited bool
	// ignoredInherited is the same question for a signal: an ignore this
	// runner inherited is listed differently from one it set itself.
	ignoredInherited bool
}

// localizeTrap records what a condition holds, just before a `trap` command
// changes it, where the dialect scopes a function's traps to the call.
//
// Called for every condition of every modification, including a `trap -`
// reset — measured, a reset is localized like a set, and the displaced
// handler comes back at the return.
//
// Three things stop it, and each is a measured row rather than a guard:
//
//   - the dialect is not scoping traps, which is every shell's default;
//   - there is no function call to be local to. A trap set at the top level
//     stays set, and so does one a sourced file sets there;
//   - the condition is EXIT, which is already scoped to the function in the
//     shell that has this option, by a rule of its own that asks no option at
//     all (ExitTrapIsFunctionLocal). Measured both ways: with the option on
//     and with it off, an EXIT trap set in a function fires at the return and
//     the caller's comes back either way. Leaving it out is also what keeps
//     the two mechanisms from racing — the restore below runs before that
//     one looks at whether this call installed a trap of its own.
func (r *Runner) localizeTrap(name string, sig syscall.Signal) {
	if r.sem().FunctionLocalTraps != TrapsGoBackAtTheReturn || name == "EXIT" {
		return
	}
	sc := r.ownScope()
	if sc == nil {
		return
	}
	// First save wins: a body that moves the same condition twice goes back
	// to what it found rather than to what it set first. Measured.
	if _, done := sc.savedTraps[name]; done {
		return
	}
	if sc.savedTraps == nil {
		sc.savedTraps = map[string]savedTrapState{}
	}
	s := savedTrapState{name: name, sig: sig}
	if slot := r.pseudoTrapSlot(name); slot != nil {
		s.pseudo, s.action = true, *slot
		s.frame, s.inherited = r.pseudoTrapOrigin(name)
	} else if action, ok := r.trapTable()[name]; ok {
		s.action = &action
		s.ignoredInherited = r.inheritedIgnored[name]
	}
	sc.savedTraps[name] = s
}

// ownScope is the innermost function scope *this* runner pushed, or nil.
//
// A subshell is a clone that shares the scope stack it was made inside, so
// asking for the innermost scope alone would hand a `( … )` written in a
// function body the caller's scope to write on. That is not a tidiness
// point: a subshell starts with the parent's handled traps back at their
// defaults, so a save taken there records "nothing was set" for a condition
// the parent is handling, and the parent's return would then clear a trap it
// never touched. Measured, `trap 'echo O' USR1; f() { ( trap 'echo S' USR1 )
// }; f; trap` still lists the outer trap.
func (r *Runner) ownScope() *scope {
	if len(r.scopes) == 0 {
		return nil
	}
	sc := r.scopes[len(r.scopes)-1]
	if sc.owner != r {
		return nil
	}
	return sc
}

// pseudoTrapOrigin is where a pseudo-condition's trap was set and whether it
// arrived from outside a subshell — the two records that travel with the
// action.
func (r *Runner) pseudoTrapOrigin(name string) (frame int, inherited bool) {
	switch name {
	case "ERR":
		return r.errTrapFrame, r.errTrapInherited
	case "DEBUG":
		return r.debugTrapFrame, r.debugTrapInherited
	case "RETURN":
		return r.returnTrapFrame, r.returnTrapInherited
	}
	return 0, false
}

// setPseudoTrapOrigin puts those two records back.
func (r *Runner) setPseudoTrapOrigin(name string, frame int, inherited bool) {
	switch name {
	case "ERR":
		r.errTrapFrame, r.errTrapInherited = frame, inherited
	case "DEBUG":
		r.debugTrapFrame, r.debugTrapInherited = frame, inherited
	case "RETURN":
		r.returnTrapFrame, r.returnTrapInherited = frame, inherited
	}
}

// restoreLocalTraps puts back everything this call displaced.
//
// Unconditional: the question was asked and answered at each modification,
// and nothing is asked again here. Measured, and it is where this parts
// company with the option table the same shell scopes — that one is asked at
// the *return*, so a body that stops asking for the scoping keeps what it
// moved, while a body that stops asking for *this* still has its traps put
// back.
func (r *Runner) restoreLocalTraps(sc *scope) {
	for _, name := range sortedTrapNames(sc.savedTraps) {
		s := sc.savedTraps[name]
		if s.pseudo {
			*r.pseudoTrapSlot(s.name) = s.action
			r.setPseudoTrapOrigin(s.name, s.frame, s.inherited)
			continue
		}
		// Through the same door a `trap` command writes, so an arrangement
		// with the operating system is made and unmade the same way: what
		// comes back is the disposition and not only what a listing shows.
		r.trapSignal(s.name, s.sig, s.action)
		if s.ignoredInherited && r.inheritedIgnored != nil {
			r.inheritedIgnored[s.name] = true
		}
	}
}

// sortedTrapNames orders the restore, so a run that puts several conditions
// back does it the same way twice.
func sortedTrapNames(m map[string]savedTrapState) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
