// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// readonlyFreezesACallLocal is `readonly` in the shell that gives the value to
// the caller and keeps the freeze for the call — see
// Semantics.ReadonlyFreezeStaysWithTheCall. It reports whether it took the
// operand; where it did not, the builtin's ordinary freeze runs, and r.unspecified
// says whether that is because the dialect could not answer.
//
// The scope is the one `typeset` declares into, asked through the same gate
// shadowTypeset asks: a function defined without the `function` word has none
// in the dialect that needs the word, and there the freeze is the shell's own,
// measured — `f() { readonly x; }; f; x=1` is refused in the shell that answers
// yes here. A name the call has already declared is not taken either: the
// freeze lands on the local the call owns, which is what the ordinary path does
// with it (`typeset x=3; readonly x=5` holds 5 and refuses the next write).
//
// The order is the measured one. The value goes first, as an ordinary
// assignment to the name the function sees — so it reaches the caller, and is
// refused where the caller's name is frozen — and only then is the local made,
// holding nothing. The function reads its own empty local afterwards:
// `readonly x=5; echo "${x-u}"` writes `u`.
func (r *Runner) readonlyFreezesACallLocal(name, value string, hasValue, appends bool) bool {
	// The scope `typeset` would declare into, which from a function without
	// the keyword is the keyword call it runs in (#6488).
	defer r.declareInTheKeywordCallersScope()()
	if len(r.scopes) == 0 || r.sem().ReadonlyDeclaresALocal != No {
		// No scope to take, or the dialect's `readonly` declares one of its
		// own (or has not said, which readonlyDeclaresALocal reports).
		return false
	}
	if !r.ask(r.sem().ReadonlyFreezeStaysWithTheCall, "`readonly` inside a function freezing a local of its own") {
		return false
	}
	sc := r.scopes[len(r.scopes)-1]
	if !sc.keyword &&
		r.ask(r.sem().TypesetLocalNeedsKeywordFunction, "`typeset` needing a keyword-defined function to declare a local") {
		return false
	}
	if r.unspecified {
		return false
	}
	base, subs, subscripted := name, []string(nil), false
	if strings.IndexByte(name, '[') > 0 {
		base, subs, subscripted = r.operandSubscripts("readonly", name)
		if !subscripted {
			// A malformed operand is the ordinary path's to refuse, in its
			// own words.
			return false
		}
	}
	if rec := sc.record(base); rec != nil && rec.holds(recValue) {
		return false
	}
	if hasValue {
		if subscripted {
			r.declareElement(base, subs[:len(subs)-1], subs[len(subs)-1], value, appends, declareFlags{}, false)
		} else if appends {
			r.declarationAppend(base, value, false, false)
		} else {
			r.setVarAs(base, value, assignedByDeclaration)
		}
		if r.unspecified || r.ctl == controlExit || r.operandGaveUpTheBuiltin() {
			return true
		}
	}
	fresh, _ := r.shadowTypeset(base)
	// The local is a scalar whatever the caller's name was: measured, `x=(a
	// b); function f { readonly x; typeset -p x; }` lists `typeset -r x`,
	// and the caller's array is back whole after the call.
	r.dropTheOuterCompound(base, fresh)
	r.declareEmpty(base, fresh, true, true, true, r.LocalInheritsTheOuterValue(), true)
	if r.unspecified || r.ctl == controlExit {
		return true
	}
	r.markReadonly(base)
	return true
}
