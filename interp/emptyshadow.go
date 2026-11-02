// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// A declaration operand that makes a local over a name that has nothing to
// shadow.
//
// `local a b c`, `local a=$1`, and `typeset a b c` or `declare a=$1` wherever
// those declare a local: most of what a function declares is an ordinary
// name with no letters on the line and nothing standing under it — no value,
// no attribute, no entry in any table of the shell's. The full road asks about
// twenty-five questions of every operand anyway — a kind fixed on the name, a
// private scope, a reference to follow, a freeze to refuse or keep, a hidden
// shadow, a tie to mirror, the export to inherit, the compound to drop, each
// letter's attribute — and the shadow itself reads and clears a dozen
// attributes and tables to save what turns out to be nothing. Every one of
// them answers "nothing to do", and that was most of the ~600 ns each name
// cost in zsh against about 70 in zsh 5.9.2 (#6373).
//
// shadowsNothing settles every one of those questions before anything is
// written, and takeAnEmptyShadow then writes what Runner.shadow writes when
// each of its reads finds nothing: the same records, each saying "there was
// none". The store is the full road's own — setVarAs for a value, under the
// form that road's store uses, and declareEmpty for none — so what a
// declaration stores is not decided here at all. The scope's return is
// untouched: restoreShadowedName reads the records as it reads any others.
//
// Two roads reach it, and the route is one for both: biLocal, and
// declareNames where typesetDeclaresALocalHere says the line makes locals —
// a `function`-form body in ksh93, any function in bash and zsh. A second
// copy of these conditions for the second road is exactly what would drift,
// so each road asks only its own line-level question (plainLocalLine,
// plainTypesetLine) and the per-operand one is shared.
//
// The conditions are the full road's own facts, asked through the helpers that
// road asks them through where one exists: storesPlainly for every question
// the store asks, Runner.shadowIsHidden for the hidden shadow, bornWith for
// the environment, the payload tables captureAttributes reads. That is also
// the hazard: a question added to the full road tomorrow is not asked here.
// TestAnEmptyShadowAnswersAsTheFullRoadDoes and
// TestAnEmptyTypesetShadowAnswersAsTheFullRoadDoes stand against it: each runs
// declarations over every state the route must either answer as the full road
// does or leave to it, with the route on and off, and requires the same
// output. The records written are shadow's own by construction, since both
// write the named sets freshShadowRecords and compoundShadowRecords.

// emptyShadowShortcut switches the route on. A variable so a test can run the
// same script with the full road answering every operand.
var emptyShadowShortcut = true

// plainLocalLine reports whether a `local` line may take the route at all:
// no letters, a scope to declare into, and nothing the line or the runner is
// in the middle of that makes a binding anything but ordinary. Asked once a
// line; shadowsNothing asks the rest of each operand.
func (r *Runner) plainLocalLine(f declareFlags) bool {
	// The private word marks every binding it makes (privateShadowTaken), and
	// a dialect whose locals start as the outer binding answers the store
	// from what stands outside — see localinherit.go.
	return emptyShadowShortcut && f == (declareFlags{}) && len(r.scopes) > 0 &&
		!r.declaringPrivate && !r.LocalInheritsTheOuterValue()
}

// plainTypesetLine is plainLocalLine for a line declareNames runs — `typeset`,
// `declare` — which makes locals only where typesetDeclaresALocalHere says it
// does: never at the top level, never under `-g` (a letter, which
// plainLocalLine already refuses), and in ksh93 only in a `function`-form
// body, where a POSIX-form `f()` assigns the caller's name instead.
//
// Asked only where the dialect has answered the axis that question may ask.
// The full road asks it per operand, at the shadow; asked here for a dialect
// that left it open, the report would come out ahead of the line's own
// refusals and then a second time beside the full road's.
func (r *Runner) plainTypesetLine(f declareFlags) bool {
	return r.plainLocalLine(f) && r.sem().TypesetLocalNeedsKeywordFunction != Unspecified &&
		r.typesetDeclaresALocalHere()
}

// shadowsNothing reports whether a declaration of name in sc on a line
// plainLocalLine accepted is one whose every question answers "nothing to
// do": an ordinary name holding nothing, carrying nothing, in no table of the
// shell's, and not yet shadowed by this scope. It writes nothing, so a no
// leaves the operand to the full road exactly as it found it.
func (r *Runner) shadowsNothing(sc *scope, name string) bool {
	// Every question the store asks of the name and of what the runner is in
	// the middle of — an identifier, none of the names the store treats
	// specially, no registered guard, refusal, writer, tie or fixed kind, no
	// reference anywhere, no float or width record. The registered names are
	// also every name a kind letter, the private scope, the tie and the
	// value refusal ask about, so those four gates answer no for it as well.
	if !r.storesPlainly(name, 0) {
		return false
	}
	// Nothing standing under the name: no value of any kind, no producer,
	// and no attribute — the freeze, the export record, `unset`'s removal,
	// the hide letter and the compound records all live in the cell.
	if c := r.cell(name); c != nil && c.live() {
		return false
	}
	// Not shadowed by this scope already, for any reason: a second
	// declaration is not fresh, and a record a tie's mirror or an `unset`
	// left is the full road's to read.
	if rec := sc.record(name); rec != nil && rec.has != 0 {
		return false
	}
	// Not in the environment either, which is a value the cell does not
	// hold and the export attribute a local may inherit (Runner.isExported).
	if _, born := r.bornWith(name); born {
		return false
	}
	// The payload tables captureAttributes reads that storesPlainly does
	// not, and what a produced parameter was last assigned.
	if _, ok := r.integerBase[name]; ok {
		return false
	}
	if _, ok := r.floatExact[name]; ok {
		return false
	}
	if _, ok := r.caseListing[name]; ok {
		return false
	}
	if _, ok := r.assigned[name]; ok {
		return false
	}
	// An array literal for the name on this line, a local that keeps the
	// outer value, and a frozen name in restricted mode, each a road of its
	// own.
	if r.literalOperands[name] || r.localKeepsOuter[name] || (r.restricted && r.restrictedFrozen[name]) {
		return false
	}
	// A live assignment prefix holding the name, whose entry the shadow
	// takes over (Runner.prefixEntryShadowed), and a compound's members,
	// which the shadow displaces with it (shadowCompoundNamespace).
	if slices.Contains(r.prefixHeldNames, name) || slices.Contains(r.functionPrefixNames, name) ||
		(r.memberNamesInUse && isMemberParent(name)) {
		return false
	}
	// A shadow over a name an enclosing scope hid is a hidden one, which
	// suspends a producer and is recorded as such.
	return !r.shadowIsHidden(name)
}

// takeAnEmptyShadow is Runner.shadow for a name shadowsNothing accepted: the
// records shadow writes when there is nothing to save, each holding "there was
// none".
func (sc *scope) takeAnEmptyShadow(name string) {
	rec := sc.recordFor(name)
	*rec = shadowRecord{name: name, has: freshShadowRecords | compoundShadowRecords}
	sc.made |= freshShadowRecords | compoundShadowRecords
}

// declareOverNothing is the rest of the full road for an operand
// shadowsNothing accepted: the shadow, and the store the road makes once every
// question has answered no. The store is the road's own call in both cases,
// and form is the one that road's store passes: biLocal's is setVar's,
// declareNames' is assignedByDeclaration.
func (r *Runner) declareOverNothing(sc *scope, name, value string, hasValue bool, f declareFlags, asked lettersAsked,
	form assignForm,
) {
	r.emptyShadows++
	sc.takeAnEmptyShadow(name)
	if hasValue {
		r.setVarAs(name, value, form)
		return
	}
	born := r.nothingStandsUnder(name, true)
	r.declareEmpty(name, true, f.export || f.readonly, asked.any, f.leavesAnAttribute(),
		f.inherit || r.LocalInheritsTheOuterValue(), false)
	r.markADeclaredNameForAllexport(name, born, true)
}
