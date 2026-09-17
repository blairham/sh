// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `unset` of a name a *calling* function made local, which is one of the two
// questions in the builtin that scoping decides and the one the panel splits
// on. See Semantics.UnsetRemovesAnEnclosingLocal for the rows.
//
// The reading this engine had is the one three of the four columns give: the
// name goes out of the variable table, the scope that declared it still holds
// its copy, and the copy comes back when that call returns. bash removes the
// *binding* instead, so what the declaration displaced shows through at once
// and goes on showing through after the owning call has returned.
//
// One binding and not every one of them. With two scopes holding the name,
// `h(){ unset v; }` called from `g(){ local v=G; h; }` called from
// `f(){ local v=F; g; }` leaves `F` visible in bash — so what is removed is
// the innermost *enclosing* shadow, and a second `unset` reaches the next one
// out. That is what makes this an undo of one scope's record rather than a
// sweep, and the implementation is literally the undo: restoreShadowedName is
// the same function popScope runs for a name when the call it belongs to
// ends, called early and for one name.
//
// Undoing the whole record and not the value alone is measured, and three
// rows say so:
//
//	v=GLOBAL; g(){ unset v; v=NEW; }; f(){ local v=L; g; }; f
//	                            leaves `v` at NEW when the script resumes —
//	                            `g`'s assignment reached the *global*, so
//	                            `f` no longer had a binding for it to find
//	declare -i v=5; f(){ local -i v=9; g; }; g(){ unset v; v=3+4; }
//	                            `7` — the outer binding's integer attribute
//	                            came back with the outer binding
//	g(){ unset v; local v=GL; }; f(){ local v=F; g; }; f
//	                            `f` reads the global afterwards, not `F`:
//	                            `g` declaring its own local after the unset
//	                            shadows the global and puts the global back
//
// A local of the scope that is *running* is the other half of the question
// and is not this: `f(){ local v=L; unset v; }` leaves the name unset and
// still local in every column, bash included, so the shadow stays and the
// ordinary removal below happens. That is the control that keeps this from
// being "what `unset` does to a local".

// unsetTakesAnEnclosingLocal takes a caller's local away where the dialect
// says `unset` removes the binding, and reports whether it did — in which
// case the name has not gone anywhere and the ordinary removal must not run.
//
// The axis is asked only where the two readings differ, which is where a
// calling scope shadowed the name and the running one did not. Everywhere
// else — at the top level, inside the scope that declared it, for a name no
// scope has touched — every column agrees and nothing is asked.
func (r *Runner) unsetTakesAnEnclosingLocal(name string) bool {
	sc, ok := r.enclosingShadowOf(name)
	if !ok {
		return false
	}
	takes := r.ask(r.sem().UnsetRemovesAnEnclosingLocal,
		"`unset` of a name a calling function made local taking the binding away")
	if r.unspecified {
		// The refusal is the answer, and the name stays where it is: a shell
		// that had just said it did not know which reading to take and then
		// removed the name anyway would have chosen one.
		return true
	}
	if !takes {
		return false
	}
	r.restoreShadowedName(sc, name)
	return true
}

// enclosingShadowOf is the scope a *calling* function's declaration of the
// name belongs to: the innermost scope holding a shadow of it, reported only
// when that scope is not the one running.
//
// The innermost and then the test, rather than a search that skips the
// running scope: a name the running scope declared is the running scope's,
// whatever an outer one also holds, and a search that walked past it would
// take a caller's binding away for `f(){ local v=L; unset v; }` — which every
// shell in the panel answers by leaving the local unset in place.
func (r *Runner) enclosingShadowOf(name string) (*scope, bool) {
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if !r.scopes[i].shadows(name) {
			continue
		}
		return r.scopes[i], i < len(r.scopes)-1
	}
	return nil, false
}

// UnsetRemovesAnEnclosingLocal reports whether `unset` of a name a calling
// function made local takes the binding away — Semantics.UnsetRemovesAnEnclosingLocal
// read back, so that a dialect with a name for the question can move it.
//
// A read of the axis rather than a second piece of state, for the reason
// ErrExitEntersACommandSubstitution is: bash's `shopt localvar_unset` and the
// default it deviates from are one thing there, so keeping a bit beside the
// axis would mean keeping the two in step and drifting in whichever direction
// nobody tested.
//
// The senses are opposite, which is the one thing to get right at the call
// site: the option *names* the suppression — `shopt -s localvar_unset` asks
// for the answer this axis calls No — so the dialect inverts, in the single
// place that knows the name. See dialect/bash's shoptSwitches.
func (r *Runner) UnsetRemovesAnEnclosingLocal() bool {
	return r.sem().UnsetRemovesAnEnclosingLocal == Yes
}

// SetUnsetRemovesAnEnclosingLocal moves it, for a dialect naming the switch.
// It travels both ways: `shopt -u localvar_unset` puts bash back where it
// starts.
func (r *Runner) SetUnsetRemovesAnEnclosingLocal(on bool) {
	a := No
	if on {
		a = Yes
	}
	r.swapSemantics(func(s *Semantics) {
		s.UnsetRemovesAnEnclosingLocal = a
	})
}
