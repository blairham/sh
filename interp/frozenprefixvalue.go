// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// A frozen name in an assignment prefix is refused, and four of the five
// columns evaluate what it was being given **first** and report that instead.
// See Semantics.FrozenPrefixIsCheckedBeforeItsValue for the panel.
//
// The external route has always done this, because it expands every value on
// its way to building the child's environment and reaches the refusal
// afterwards — so `readonly r=1; r=$((1/0)) /bin/echo RAN` wrote the division
// here and in the reference alike. The builtin and function routes skip a
// frozen name's value before anything expands it, so bash's answer fell out of
// a gap on those two rather than out of the axis (#4685).

// expandTheFrozenPrefixValues evaluates the value of every frozen name in this
// prefix ahead of the refusal, and reports whether one of them would not
// expand.
//
// Asked only where there **is** a frozen name, so an ordinary prefix asks
// nothing and a dialect with no answer refuses only the line the question is
// about.
//
// It stops at the first value that fails, which is the walk's own rule: the
// entries behind a value that would not expand are not expanded, unanimously
// in the panel. The caller withholds the refusal when this reports true — the
// four columns that evaluate first write the expansion's sentence and never
// the frozen name's.
func (r *Runner) expandTheFrozenPrefixValues(assigns []*syntax.Assign) bool {
	if len(r.frozenPrefixNames(assigns)) == 0 {
		return false
	}
	if r.frozenPrefixCheckedBeforeItsValue() {
		return false
	}
	if r.unspecified {
		// Nobody answered. Evaluating either way would answer it, so the
		// value is left alone and the caller's refusal stands beside the
		// refusal this already wrote.
		return false
	}
	for _, a := range assigns {
		if a.Operand {
			continue
		}
		if _, ok := positionalAssignIndex(a.Name); ok {
			// A positional parameter is not one of the names
			// frozenPrefixNames counted, so it is not one of the values this
			// is about either — and the routes expand it for themselves.
			continue
		}
		if !r.readonly[a.Name] {
			continue
		}
		// Located the way any other prefix value's failure is, which is a
		// question one column answers by the command — see
		// interp/prefixfailurelocation.go. Armed here as well as in
		// prefixAssignValue because this is a road to the same expansion.
		put := r.speakForAPrefixValue()
		r.expandWord(a.Value)
		put()
		if r.expandErr || r.ctl != controlNone {
			return true
		}
	}
	return false
}

// frozenPrefixCheckedBeforeItsValue asks the axis. One spelling, because the
// **external** route reaches the same question from the other side: it expands
// every value on its way to building the child's environment and meets the
// refusal afterwards, so what it needs is whether to skip the expansion rather
// than whether to make one early.
func (r *Runner) frozenPrefixCheckedBeforeItsValue() bool {
	return r.ask(r.sem().FrozenPrefixIsCheckedBeforeItsValue,
		"a frozen name in a prefix refused before its own value is evaluated")
}

// frozenPrefixValueIsSpentForAChild reports whether the route that hands a
// prefix to a child must leave a frozen name's value alone: either the early
// check has already expanded it, or this dialect never evaluates it.
//
// Asked only where there is a frozen name, so an ordinary prefix in front of
// an external command asks nothing.
func (r *Runner) frozenPrefixValueIsSpentForAChild(assigns []*syntax.Assign) bool {
	if r.prefixCheckedFirst {
		return true
	}
	if len(r.frozenPrefixNames(assigns)) == 0 {
		return false
	}
	return r.frozenPrefixCheckedBeforeItsValue()
}
