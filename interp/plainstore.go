// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A bare assignment that stores a value and does nothing else.
//
// `x=`, `x=0`, `x=$y`: most of the assignments a script runs put a value in an
// ordinary variable, and nothing about them is a question. The full road asks
// about thirty anyway — a reference to follow, a freeze, a hidden kind to
// watch, a compound to retype, a discipline to run, an attribute to fold the
// value through, a width to learn, a tie to mirror, a producer to feed, a
// guard and an action registered for the name, a frame selected elsewhere —
// and every one answers "nothing to do". That was about half of what `x=`
// cost: 0.28 µs against zsh's 0.04 in the zsh dialect, where the variable is
// written in a few nanoseconds once the questions are over (#6423).
//
// storesAPlainScalar settles every one of those questions before anything is
// expanded, and storePlainScalar then does what the full road does when each
// of them answers no: the scope lint, if it is on, the store, and
// allexport. Anything it is not sure of goes to the
// full road having read nothing, the same arrangement plainParamValue has.
//
// The conditions are the full road's own, read from its code rather than
// from a list of what seemed likely. That is also the hazard: a question
// added to the full road tomorrow is not asked here. Two tests stand against
// that. TestAPlainScalarStoreAnswersAsTheFullRoadDoes runs scripts that reach
// each question with the shortcut on and off and requires the same output,
// and TestTheFullRoadOfAnAssignmentIsTheOneThePlainStoreWasCheckedAgainst
// fails when the calls the full road makes change, which is the moment this
// file has to be read again.

// plainScalarStoreShortcut switches the shortcut on. A variable so a test can
// run the same script with the full road answering every assignment.
var plainScalarStoreShortcut = true

// storesAPlainScalar reports whether the assignment is a plain store that
// storePlainScalar may make in the full road's place.
func (r *Runner) storesAPlainScalar(a *syntax.Assign) bool {
	if !plainScalarStoreShortcut || a.Index != nil || a.IsArray || a.Members != nil || len(a.Leading) > 0 ||
		a.Member != "" || a.Append || a.EmptySubscript || a.Operand || a.IndexFlags != nil {
		return false
	}
	name := a.Name
	// An identifier, so not a positional parameter and not a compound
	// member; and none of the names the store itself treats specially.
	if !identifierName(name) || name == "OPTIND" || name == "PATH" || name == r.sem().IgnoredNamesVariable {
		return false
	}
	// What the runner is in the middle of: a traced assignment's value is
	// expanded by the caller, and each of the rest is a road of its own.
	if r.expanded != nil || r.unspecified || r.namespace != "" || r.frameSelected ||
		r.storingSelfNameref != "" || r.retypingFrozen != "" || r.declaringWithoutAValue ||
		r.viewIsAlreadyFolded || r.mirroring || r.caseFoldHeld != "" || r.privateDeclared ||
		len(r.privateShield) > 0 || len(r.nameref) > 0 || r.scalarAssignmentGlobs() {
		return false
	}
	if r.LocaleCharset != nil {
		for _, v := range localeVariables {
			if v == name {
				return false
			}
		}
	}
	// The name holds nothing but a scalar, carries no attribute, and no
	// table of the shell's has an entry for it.
	if c := r.cell(name); c != nil && (c.has&^slotScalar != 0 || c.attrs != 0) {
		return false
	}
	if r.disciplined[name] || r.compoundVariable[name] || r.capitalized[name] {
		return false
	}
	// The tables the shell registers its own names in. One test of a filter
	// answers for all seven where the name was never registered, which is
	// nearly every name a script writes.
	if r.specialNames.mayHold(name) {
		if r.deferredParams[name] {
			return false
		}
		if _, ok := r.kindFixed[name]; ok {
			return false
		}
		if _, ok := r.assignmentGuards[name]; ok {
			return false
		}
		if _, ok := r.assignmentActions[name]; ok {
			return false
		}
		if _, ok := r.dynamicWriters[name]; ok {
			return false
		}
		if _, ok := r.dynamicArrayWriters[name]; ok {
			return false
		}
		if _, ok := r.tied[name]; ok {
			return false
		}
	}
	if _, ok := r.floatPrecision[name]; ok {
		return false
	}
	if _, ok := r.fieldWidth[name]; ok {
		return false
	}
	return r.valueOnlyReads(a.Value)
}

// valueOnlyReads reports whether expanding w can do nothing but read: no
// substitution of any kind, and no `~` or `=` for the tilde and equals passes
// to act on. A read of a plain `$name` is admitted where plainParamValue
// answers it, which is a read of an ordinary scalar and nothing else.
func (r *Runner) valueOnlyReads(w *syntax.Word) bool {
	if w == nil {
		return true
	}
	for i := range w.Spans {
		s := &w.Spans[i]
		if s.Kind != syntax.Literal || strings.ContainsAny(s.Value, "~=") {
			if len(w.Spans) == 1 {
				if r.plainTrimOf(s) {
					return true
				}
				_, ok := r.plainParamValue(w)
				return ok
			}
			return false
		}
	}
	return true
}

// storePlainScalar is the full road for an assignment storesAPlainScalar
// accepted, with every question answered: what is left is the lint, the store
// and allexport.
func (r *Runner) storePlainScalar(a *syntax.Assign) {
	value, ok := r.plainParamValue(a.Value)
	if !ok {
		value = r.expandAssignValue(a.Value)
	}
	if !r.scopeWarningsAreOff() {
		r.warnAboutTheScope(a.Name, scopeWarningScalar)
	}
	// The full road clears the declared-empty and unset marks here; both are
	// attributes in the cell now (#6442), and a name storesAPlainScalar
	// accepted carries none.
	r.varsPut(a.Name, value)
	r.markForAllexport(a.Name)
}

// plainTrimOf reports whether a value's one span is a trim of an ordinary
// scalar by written text — `${s#*/}`, `${s%%.*}` — which reads and does
// nothing else.
//
// The value is still expanded by the full road (storePlainScalar falls back
// to expandAssignValue), so nothing here decides what the trim answers. What
// this settles is that expanding it cannot change any of the answers
// storesAPlainScalar has already taken about the name being written: the
// subject is a name plainParamValue would read, in a state where it would
// read it, and the operand is text with nothing in it to expand. Two things a
// trim's operand can do besides match are left to the full road by name. A
// `(#b)` or `(#m)` flag publishes `$match` and `$MATCH`, which a script can
// be assigning — measured on zsh 5.9.2, `s=ab; match=${s#(#b)(a)}` leaves
// `typeset match=b`, where taking the plain store here left the array the
// publish wrote. And a `~` reaches the tilde pass, and a `~(…)` group the
// matcher's own reading of one.
//
// The record a pattern match writes where `=~` writes one is not on that
// list: it is written to a name no assignment spells (`.sh.match`), and a
// reference that could alias it already sends every store to the full road.
//
// This is the trim row of #6353's construct table: before it, `x=${s#*/}`
// asked the full road's thirty questions about `x` on every iteration.
func (r *Runner) plainTrimOf(s *syntax.Span) bool {
	if s.Kind != syntax.ParamExp || s.Quoting != syntax.Unquoted || s.Param == nil || !r.readsAreOnlyReads() {
		return false
	}
	e := s.Param
	switch e.Op {
	case syntax.ParamTrimPrefix, syntax.ParamTrimPrefixLong, syntax.ParamTrimSuffix, syntax.ParamTrimSuffixLong:
	default:
		return false
	}
	if e.Arg == nil || !bareApartFromTheOperator(e) || !r.ordinaryScalarName(e.Name) {
		return false
	}
	var last byte
	for i := range e.Arg.Spans {
		a := &e.Arg.Spans[i]
		if a.Kind != syntax.Literal || strings.IndexByte(a.Value, '~') >= 0 ||
			strings.Contains(a.Value, "(#") || (last == '(' && strings.HasPrefix(a.Value, "#")) {
			return false
		}
		if a.Value != "" {
			last = a.Value[len(a.Value)-1]
		}
	}
	return true
}
