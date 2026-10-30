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

// expandThePrefixUpToTheFrozenName works through the prefix in the order the
// script wrote it, as far as the first frozen name, and reports whether one of
// those values would not expand.
//
// **The refusal happens at the entry it is about**, which is what this walk is
// for: the entries in front of a frozen name have already expanded when it is
// refused, and the entries behind it have not. The frozen name's own value is
// expanded here too in the four columns that evaluate it before refusing the
// name — see Semantics.FrozenPrefixIsCheckedBeforeItsValue — and the caller
// withholds the refusal when that expansion is what failed.
//
// This named every frozen name in a pass of its own ahead of the walk, so the
// complaint came out in front of every value rather than between the two
// entries it stands between. Measured 2026-09-27 from a script file under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, `readonly r=1` on the line
// before and `echo "st=$?"` on the line after:
//
//	a=$(echo A >&2) r=$(echo R >&2) echo RAN
//
//	bash 5.3.20   A, the refusal, RAN
//	dash 0.5.12   A, R, the refusal
//	zsh 5.9.2     A, R, the refusal
//	ksh93u+       A, R, RAN
//
// And written the other way round — `r=…` first — every column writes what it
// writes for the frozen entry and *then* `A`, which is the control that holds
// the refusal fixed and moves what is in front of it. A prefix with nothing
// frozen in it agrees on every row in every column, and so does a single
// frozen entry with nothing in front of it, which is why this was invisible
// until a second entry was put beside it (#4783).
//
// Three further rows say what the walk is, rather than only that it is
// ordered. A value in front of the frozen name that will not expand stops the
// walk and the refusal is **never written** — `a=$((1/0)) r=2 echo RAN` is the
// division alone in all four. The values are held for the entries behind them
// as they expand, exactly as the two other walks hold them, so
// `a=1 b=${a}X r=2 cmd` reads `b` from this walk's own `a`. And a refused
// subscripted word is left where it stands rather than expanded here: its
// refusal belongs to the ordered walk that owns it.
//
// Asked only where there **is** a frozen name, so an ordinary prefix walks
// nothing here and a dialect with no answer refuses only the line the question
// is about.
//
// refuseEach is nil on every route but one. Where it is set, the walk does not
// end at the first frozen entry: it refuses each one through refuseEach where
// it reaches it and carries on expanding what is behind it, which is the walk
// of a shell that names every frozen entry and still runs the command. See
// Runner.refusePrefixesEarly for the measurement (#6546).
//
// traceEach says this command's prefix is written a line per entry ahead of
// the command, so each entry in front of the frozen one is written as it
// expands, with its subscript read in turn the way the ordered walk reads it,
// and the refusal lands behind those lines. Measured 2026-10-30 on ksh93u+
// 2012-08-01 from `-c` and script files under `env -i PATH=/usr/bin:/bin`,
// `readonly r=1` first:
//
//	set -x; x=1 r=2 y=3 :            `+ x=1`, then the refusal
//	set -x; x=1 r=2 y=3 f            the same, `f` a POSIX-form function
//	set -x; x=1 r=$(echo s >&2) y=3 :
//	        `+ x=1`, the substitution's lines, then the refusal
//	typeset -A a; set -x; a[""]=1 r=2 a[q]=3 :
//	        the empty-key warning, `+ a['']=1`, then the refusal
//
// where this refused ahead of every line in that column (#6987). bash 5.3.20
// writes its entries the same way, which is where the per-entry lines here
// were first measured (#5546).
func (r *Runner) expandThePrefixUpToTheFrozenName(
	assigns []*syntax.Assign,
	refuseEach func(name string),
	traceEach bool,
) bool {
	if len(r.frozenPrefixNames(assigns)) == 0 {
		return false
	}
	evaluatesTheValue := !r.frozenPrefixCheckedBeforeItsValue()
	if r.unspecified {
		// Nobody answered. Expanding anything here would answer it, so the
		// walk is not made and the caller's refusal stands beside the
		// refusal the ask already wrote.
		return false
	}
	// What this walk has expanded so far, visible to the entries behind it
	// and put back before any route stores one for real. The same helper the
	// ordered walk and the trace walk use, for the same reason — see
	// interp/prefixsees.go.
	var held heldPrefix
	defer held.release(r)
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
		frozen := r.prefixEntryIsFrozen(a)
		if frozen && !evaluatesTheValue {
			// The name is refused without its value being looked at, which
			// is where this walk ends: the caller writes the refusal next.
			if refuseEach == nil {
				return false
			}
			refuseEach(a.Name)
			continue
		}
		if !frozen && !r.prefixEntryHasATraceableValue(a) {
			// A subscripted word the dialect refuses, whose value is never
			// expanded and whose refusal is the ordered walk's to write, or
			// the splice form, which has no one value.
			continue
		}
		// An element write's subscript, read where the entry stands, in the
		// walk that writes the entry's line: its words ahead of the value
		// where the dialect reads them first, and its arithmetic and any
		// warning the store owes once the value is in. See
		// interp/prefixsubscriptinturn.go. Not stored here, because the
		// command is about to be refused, so no warning is owed to a store.
		inTurn := false
		if !frozen && traceEach {
			var expanded bool
			if inTurn, expanded = r.readPrefixSubscriptBeforeTheValue(a); !expanded {
				return true
			}
		}
		// Located the way any other prefix value's failure is, which is a
		// question one column answers by the command — see
		// interp/prefixfailurelocation.go. Armed here as well as in
		// prefixAssignValue because this is a road to the same expansion.
		put := r.speakForAPrefixValue()
		joined := r.recordPrefixTraceValue(a, r.prefixExpansion(a))
		put()
		if r.expandErr || r.ctl != controlNone {
			return true
		}
		if inTurn && !r.placePrefixSubscript(a, false) {
			return true
		}
		r.holdPrefixEntry(&held, a, joined)
		if !frozen && traceEach {
			// Written as it is expanded, ahead of the refusal still to come,
			// in the column that writes a line per entry in front of the
			// command: measured 2026-10-02, `readonly r; set -x; a=1 r=2
			// true` is `+ a=1`, the refusal, `+ true` in bash 5.3.20 (#5546).
			// The walk behind this one does not write it again.
			d := r.diag()
			for _, w := range r.prefixTraceWords([]*syntax.Assign{a}, d) {
				r.awaitTraceTurn()
				r.traceLine(w, d)
				r.releaseTraceTurn()
			}
			r.prefixTracedEarly = append(r.prefixTracedEarly, a)
		}
		if frozen {
			// Its value expanded and nothing behind it does: the refusal is
			// the caller's next act.
			if refuseEach == nil {
				return false
			}
			refuseEach(a.Name)
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
