// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

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
// One piece of a shadow is left standing, and it is named rather than
// implied: a `local OPTIND` also displaces the `getopts` scan position, and
// that half is put back by restoreGetoptsCursor, which is keyed on the scope
// rather than on the name and asks an axis of its own —
// Semantics.GetoptsLocalOptindRestoresTheCursor — at the scope's exit. What
// `unset OPTIND` inside a callee should do to the caller's *position inside a
// word* has not been measured, so nothing here decides it: the parameter is
// handed back as any other is, and the cursor still returns when the owning
// call does.
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
		if sc != nil {
			// The **running** scope declared it, which is the one shape
			// every column answers alike: the local is unset where it
			// stands. A call's prefix further out is not reached, because
			// this scope's binding is the innermost one and is what `unset`
			// is about.
			return false
		}
		// No scope holds the name at all, so the innermost binding — if
		// there is one — belongs to an enclosing call's assignment prefix.
		return r.unsetTakesACallPrefixBinding(name)
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

// callPrefixFrame is what one enclosing call's assignment prefix is holding.
//
// A frame rather than a flat list because `unset` has to find the *innermost*
// call holding the name, and because taking one entry out must leave the rest
// of that call's prefix to be given back as it always was.
type callPrefixFrame struct {
	names []string
	undo  []savedVar
	// scoped says the prefix wrote a cell belonging to the call rather than
	// to the shell — see Runner.prefixScopedToTheCall — which the take-back
	// needs and which is a property of the call rather than of a name.
	scoped bool
}

// cloneCallPrefixes copies the frames a subshell inherits, down to each
// frame's own slices: a name a subshell's `unset` takes out of a frame is a
// name the parent's call still has.
func cloneCallPrefixes(frames []callPrefixFrame) []callPrefixFrame {
	if frames == nil {
		return nil
	}
	out := make([]callPrefixFrame, len(frames))
	for i, f := range frames {
		out[i] = callPrefixFrame{
			names:  slices.Clone(f.names),
			undo:   slices.Clone(f.undo),
			scoped: f.scoped,
		}
	}
	return out
}

// unsetTakesACallPrefixBinding is unsetTakesAnEnclosingLocal's other place to
// look: a name an assignment prefix in front of a **function call** gave the
// shell, which is a binding like any other to the dialect that removes one.
//
// Measured 2026-09-18 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with a scratch HOME:
//
//	v=GLOBAL
//	g() { unset v; echo "  g [${v-U}]"; }
//	f() { g; echo "  f [${v-U}]"; }
//	v=PRE f; echo "global [${v-U}]"
//
//	bash 5.3.20, 3.2.57   [GLOBAL] [GLOBAL] [GLOBAL]
//	zsh 5.9.2, dash       [U]      [U]      [GLOBAL]
//
// So it is `UnsetRemovesAnEnclosingLocal` again — the same axis #3435 built
// for `local`, reached through the other mechanism — and not a second one: the
// panel splits the same way and one shell's `shopt localvar_unset` moves both.
// The global shows through for the rest of `g` *and* for the rest of `f`,
// which is what says the binding is gone rather than emptied.
//
// A prefix to a call takes no scope here, so `r.scopes` records nothing and
// the search above finds nothing to take away. The frames are where it is
// written down instead, and the **innermost** one holding the name is the one
// removed: a call two frames out keeps its own entry.
//
// Removing it means giving the name back what the prefix displaced and then
// forgetting the entry, so the call's own take-back does not put the prefix's
// value back on the way out — which would resurrect a binding the script has
// just removed.
func (r *Runner) unsetTakesACallPrefixBinding(name string) bool {
	frame := -1
	for i := len(r.callPrefixes) - 1; i >= 0; i-- {
		if slices.Contains(r.callPrefixes[i].names, name) {
			frame = i
			break
		}
	}
	if frame < 0 {
		return false
	}
	takes := r.ask(r.sem().UnsetRemovesAnEnclosingLocal,
		"`unset` of a name a call's assignment prefix gave the shell taking the binding away")
	if r.unspecified {
		// The refusal is the answer and the name stays where it is, exactly
		// as for the `local` spelling above.
		return true
	}
	if !takes {
		return false
	}
	f := &r.callPrefixes[frame]
	i := slices.IndexFunc(f.undo, func(u savedVar) bool { return u.name == name })
	if i < 0 {
		// Held and not saved, which is a prefix this dialect keeps: there is
		// nothing displaced to give back, so the ordinary `unset` runs.
		f.names = slices.DeleteFunc(slices.Clone(f.names), func(n string) bool { return n == name })
		return false
	}
	r.restoreVars(f.undo[i : i+1])
	f.undo = slices.Delete(slices.Clone(f.undo), i, i+1)
	f.names = slices.DeleteFunc(slices.Clone(f.names), func(n string) bool { return n == name })
	return true
}
