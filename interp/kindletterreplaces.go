// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A kind letter written over a name that already has another kind.
//
// One shell in the panel takes the old kind away and the rest let the two
// stand together, which is [Semantics.KindLetterReplacesTheKind] and is where
// the measured grid lives. This file is the half that does it.
//
// **What is cleared is what the name was carrying before this line**, never
// what this line's own letters write. `typeset -ia q` leaves the name an
// array *and* an integer in the replacing shell as much as in the others, so
// the rule cannot be read off the letters among themselves; it is read off
// the last kind letter and applied to the state that letter arrived at. That
// is why this runs ahead of both the numeric attributes and the container
// mark rather than between them.

// kindLetterReplacesWhatTheNameWas takes the attributes of every kind but the
// one this declaration's last kind letter names off the name, where the
// dialect replaces rather than accumulates.
//
// Nothing at all where no kind letter was written, which is most
// declarations: `typeset -x q` over an array leaves the array, measured, and
// so does a bare `typeset q`. A **plus** letter is not this either — that is
// a removal with a rule of its own, and the container half of it is
// declareFlags.containerLetterRemoved.
func (r *Runner) kindLetterReplacesWhatTheNameWas(name string, f declareFlags) {
	kind, plus, written := f.kindLetterWritten()
	if !written || plus {
		return
	}
	if !r.nameAlreadyHasAnotherKind(name, kind) {
		// **Asked only at the disagreement.** A declaration whose letter
		// names the kind the name already has, or one over a name with no
		// kind at all, is every ordinary `typeset -A m` there is — and an
		// axis consulted there would refuse by name in the substrate and in
		// every dialect that has not answered it, over a line the two shells
		// do not part on. See docs/spec/semantics.md on where an axis is
		// asked.
		return
	}
	if !r.ask(r.sem().KindLetterReplacesTheKind,
		"a kind letter written over a name that already has another kind") {
		return
	}
	if kind != ArrayParameter && kind != AssocParameter {
		// A numeric letter arriving, so any container the name was holding
		// goes. Measured: `typeset -a q; typeset -F q` is `typeset -F
		// q=0.0000000000` with the array gone, where this shell used to list
		// `typeset -aF q=(  )` — a name that was an array and rendered as a
		// float, which is not a state the reference has.
		target := r.throughNameref(name)
		if !r.arrayDeclared(target) && !r.assocDeclared(target) {
			return
		}
		delete(r.Arrays, target)
		r.dropAssocTable(target)
		// And the value the container held goes with it rather than becoming
		// the scalar the name now is: measured, `typeset -a q=(1 2); typeset
		// -i q` is `typeset -i q=0` and not `q=1`. The scalar view a stored
		// array keeps in step is what the first element would have arrived
		// through, so clearing the tables alone left the name rendering the
		// element it had just stopped holding.
		r.setVar(target, "")
		return
	}
	// And a container letter arriving, so the numeric attributes go. The
	// tables are deliberately left alone here: one container letter over the
	// other is a *conversion* with two measured answers of its own, which is
	// Semantics.TableUnderAnArrayDeclaration and its neighbor, and taking the
	// old table away here would decide that question by deletion.
	delete(r.integer, name)
	delete(r.integerBase, name)
	delete(r.floatPrecision, name)
	r.markFloatExponent(name, false)
}

// nameAlreadyHasAnotherKind reports whether this name is carrying a kind that
// the arriving letter would displace, which is the only state the axis above
// is about.
//
// The container pair is deliberately **not** one of them: one container
// letter over the other is a conversion with two measured answers of its own
// — Semantics.TableUnderAnArrayDeclaration and its neighbor — so a name that
// is an array meeting `-A` has not reached this question at all.
func (r *Runner) nameAlreadyHasAnotherKind(name string, arriving ParameterKind) bool {
	name = r.throughNameref(name)
	switch arriving {
	case ArrayParameter, AssocParameter:
		_, float := r.floatPrecision[name]
		return r.integer[name] || float
	default:
		return r.arrayDeclared(name) || r.assocDeclared(name)
	}
}
