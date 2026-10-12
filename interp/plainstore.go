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
// of them answers no: the scope lint, if it is on, the two bookkeeping
// deletes, the store, and allexport. Anything it is not sure of goes to the
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
	if r.deferredParams[name] || r.disciplined[name] || r.compoundVariable[name] || r.capitalized[name] {
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
// and its bookkeeping.
func (r *Runner) storePlainScalar(a *syntax.Assign) {
	value, ok := r.plainParamValue(a.Value)
	if !ok {
		value = r.expandAssignValue(a.Value)
	}
	if !r.scopeWarningsAreOff() {
		r.warnAboutTheScope(a.Name, scopeWarningScalar)
	}
	delete(r.declaredEmpty, a.Name)
	r.nameIsBack(a.Name)
	r.varsPut(a.Name, value)
	r.markForAllexport(a.Name)
}
