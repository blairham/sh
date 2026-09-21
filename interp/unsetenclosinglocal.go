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
//
// # What the running scope is left holding
//
// The shadow staying is not the whole of that column's answer, and the half
// that was missing is a *listing*. Measured 2026-09-21, `env -i
// PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME, from a script file, with
// `v=global` set outside:
//
//	f() { local v; unset v; typeset -p v; echo "st=$?"; echo "[${v-UNSET}]"; }
//
//	bash 5.3.20   declare -- v    st=0   [UNSET]
//	bash 3.2.57   v: not found    st=1   [UNSET]
//	zsh 5.9.2     (nothing)       st=0   [UNSET]
//	ksh93u+       (nothing)       st=0   [UNSET]
//	dash, ash     no listing to ask with
//
// So the name is unset, the outer value stays hidden, and in one column the
// binding is still *declared*: exactly the state a valueless declaration
// leaves behind, which this engine already keeps and lists — see
// baredeclaration.go. It is that record rather than a second one, so the
// question of what a listing does with it is the question that record
// already asks, and a dialect answers it once.
//
// Four shapes say the record is the bare one and carries nothing else. A
// `local -x v`, a `local -i v` and a `local v=zz` all list as `declare -- v`
// after the unset — the letters go with the value — and a second `unset` of
// the same name leaves it exactly where the first did.
//
// The record is deliberately not made for a name no scope shadows: `v=1;
// unset v; typeset -p v` is `v: not found` in that column too, and `declare
// xyz; xyz=v; unset xyz` is the same — the removal clears the record along
// with the attributes, which is what clearAttributes already does.

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
	// scopeDepth is how many scopes were open when the frame was pushed, so
	// a frame and a scope holding the same name can be told apart by which
	// took it first. Read by shellsOwnCellUnderACallPrefix, which has to
	// know whether a `local` further out owns the shell's cell or the frame
	// does. Both stacks are pushed from the one place in the one order, so
	// the number is the whole of the comparison.
	scopeDepth int
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
			names:      slices.Clone(f.names),
			undo:       slices.Clone(f.undo),
			scoped:     f.scoped,
			scopeDepth: f.scopeDepth,
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

// unsetLeavesARunningScopesLocalDeclared records that a local of the scope
// that is *running* is still declared once its value has gone.
//
// Called after the removal rather than before it, because the removal is
// what takes the record away: clearAttributes drops it with the letters, and
// a record written first would be deleted a line later.
//
// The record is baredeclaration.go's and not one of its own, for the reason
// the comment at the head of this file gives — the state is the same state,
// so the listing question is asked once and answered once per dialect.
//
// again says an earlier `unset` in this call had already left the name
// declared, which is what tells the *first* removal from a later one — see
// theRunningCallsPrefixHolds, which is the one thing the two do differently.
// Read by the caller rather than here, because the removal in between is what
// takes the record away.
func (r *Runner) unsetLeavesARunningScopesLocalDeclared(name string, again bool) {
	sc, enclosing := r.enclosingShadowOf(name)
	if sc == nil || enclosing {
		// No scope holds the name, or the one that does is a caller's —
		// which is the question unsetTakesAnEnclosingLocal already answered
		// and is not this one.
		return
	}
	setBool(&r.unsetLeftItDeclared, name, true)
	if !again && r.theRunningCallsPrefixHolds(name) {
		// The one shape where the placeholder carries a letter. See
		// theRunningCallsPrefixHolds for the rows.
		r.exported[name] = true
	}
}

// theRunningCallsPrefixHolds reports whether the binding this call's `local`
// displaced is the call's **own assignment prefix** — `foo=abc c5`, with
// `local foo` inside `c5`.
//
// It is the one shape where the placeholder an `unset` leaves carries an
// attribute. Measured 2026-09-21, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a
// scratch HOME, from a script file, on bash 5.3.20, with
// `c5() { local foo; unset foo; declare -p foo; }`:
//
//	foo=abc c5                                  declare -x foo
//	foo=abc c5, with `local -i foo`             declare -x foo
//	foo=abc c5, with `local foo=L`              declare -x foo
//	export g=G; c(){ local g; unset g; … }      declare -- g
//	declare -i n=5; c(){ local n; unset n; … }  declare -- n
//	p=P; c(){ local -x p; unset p; … }          declare -- p
//	foo=abc c, where c calls the declaring d    declare -- foo
//	foo=abc c5, unset twice                     declare -- foo
//
// So it is the *running* call's prefix and not a caller's, and the letter is
// gone once a second `unset` has been through — which is the `again` argument
// above. An exported global underneath is not this: the last three rows of
// the first block are the controls that say the placeholder is otherwise
// letter-free whatever it displaced.
//
// The letter is not only a listing. The same call's `foo=zz` afterwards is
// `declare -x foo="zz"` there and reaches a child as `foo=zz`, where a
// placeholder with no letter leaves the child reading the prefix's own value
// — see hiddenExports, which is the other half of this (#4050).
//
// The innermost frame and the depth together, because a frame and a scope
// holding the same name are told apart by which took it first: the frame is
// pushed before the call's scope, so the running call's frame is the one
// whose scopeDepth is one below the standing scope count.
func (r *Runner) theRunningCallsPrefixHolds(name string) bool {
	if len(r.callPrefixes) == 0 || len(r.scopes) == 0 {
		return false
	}
	f := r.callPrefixes[len(r.callPrefixes)-1]
	return f.scopeDepth == len(r.scopes)-1 && slices.Contains(f.names, name)
}
