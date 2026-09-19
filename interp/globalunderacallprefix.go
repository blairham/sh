// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "slices"

// A `-g` declaration written inside a call that was given an assignment
// prefix, which is a write that goes **under** the prefix rather than over
// it.
//
// The prefix and the letter each have an answer here already — the prefix is
// given back when the call ends, and the letter says the declaration takes no
// scope of its own — and putting the two together is a third thing neither of
// them says. Measured 2026-09-18 on bash 5.3.20, from a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME and no
// `set -o posix`, so none of #3447's persistence is in play:
//
//	inner(){ declare -g t=3; }
//	outer(){ t=7 inner; echo "in outer [${t-U}]"; }
//	t=9 outer; echo "top [${t-U}]"
//
//	bash 5.3.20   in outer [9]   top [3]
//	zsh 5.9.2     in outer [9]   top [U]
//
// The `9` says the **inner** call's take-back really did run, and the `3` at
// the top says the outer one gave back the value the declaration wrote rather
// than the one it had displaced. So the write did not land on either
// temporary binding: it landed on the shell's own cell, underneath both of
// them, and what the outermost take-back gave back was that cell.
//
// Two more rows say the same from the other side. A read inside the body is
// still the prefix's — `declare -g t=3; echo "$t"` prints `7` there and not
// `3` — and a `-g` two calls down from the prefix reaches it just the same.
// ksh93 has no `-g` letter at all, so the panel is two columns wide.
//
// Two shapes of it are deliberately left out and measured in #3685: an array
// literal, which reaches the shell as a command operand assigned after the
// builtin has returned rather than as part of the declaration, and a prefix
// in front of the declaration *itself*, which is the builtin's own entry and
// the innermost binding rather than an enclosing call's.
//
// **This is `DeclareGlobalReachesPastALocal` reached through the other
// mechanism, not a second axis.** The split is the same one and for the same
// reason: bash's letter names the global cell, so it writes under whatever is
// standing on the name, and zsh's letter means "take no new local", so it
// writes the binding that is visible — which here is the prefix's, and which
// leaves with it. Measured on the pair that makes the two readings say
// different things, `f(){ local x=5; declare -g x=3; }`, and the columns
// answer it exactly as they answer the prefix rows. `unset` is the precedent
// for the shape: see unsetTakesACallPrefixBinding, where one axis is likewise
// read at a `local` and at a call's prefix.

// liftedPrefix is one name's temporary binding, taken off for the length of a
// global declaration.
type liftedPrefix struct {
	name string
	// live is what the name held with the prefix on it, put back when the
	// declaration is done.
	live savedVar
	// frame and at are where the shell's own cell is kept while the prefix
	// stands over it, found again by index rather than held as a pointer: a
	// declaration can take an entry out of a frame, and a pointer into a
	// slice that has been rewritten is a pointer to the wrong entry.
	frame, at int
}

// globalDeclarationRunsOnTheShellsOwnCell takes the temporary bindings an
// enclosing call's assignment prefix has put on top of each name a `-g`
// declaration is about to write, lifts them, and hands back the function that
// puts them on again.
//
// Between the two the declaration runs against the shell's own cell and every
// step of it lands there — the value, the export attribute, the freeze, the
// integer letter, an append's left-hand side. That is the point of doing it
// once around the whole operand list rather than at each write: `declare -gx
// e=3` under a prefix leaves `declare -x e="3"` behind in bash 5.3.20 and
// `declare -gr f=3` leaves `declare -r f="3"`, so it is not the value alone
// that goes underneath, and a write-through built a field at a time would
// have had to name every field a declaration can touch.
//
// Whatever the declaration leaves on that cell is recorded as what the
// outermost take-back will give back, which is the whole of the measurement:
// the inner calls still give back what they displaced.
func (r *Runner) globalDeclarationRunsOnTheShellsOwnCell(names []string) func() {
	var lifted []liftedPrefix
	for _, name := range names {
		frame, at, ok := r.shellsOwnCellUnderACallPrefix(name)
		if !ok {
			continue
		}
		if !r.ask(r.sem().DeclareGlobalReachesPastALocal,
			"`declare -g` writing under an enclosing call's assignment prefix") {
			// Either answer leaves the prefix's binding where it is, which
			// is the column that writes what is visible; a refusal has
			// already said so and the declaration that follows is its own
			// business.
			continue
		}
		live := r.saveVar(name)
		// The shell's own cell, made visible for the length of the
		// declaration. restoreVar rather than a write, because what the
		// prefix displaced is a whole state and not a string: an array, a
		// table, the export tri-state and the attributes all came off with
		// it and all of them are what the declaration is writing over.
		r.restoreVar(r.callPrefixes[frame].undo[at])
		lifted = append(lifted, liftedPrefix{name: name, live: live, frame: frame, at: at})
	}
	if lifted == nil {
		return func() {}
	}
	return func() {
		for i := len(lifted) - 1; i >= 0; i-- {
			l := lifted[i]
			if l.frame >= len(r.callPrefixes) {
				// The call returned while its own declaration was running,
				// which nothing in the panel does; the binding has already
				// been given back and there is nothing to put a value into.
				continue
			}
			f := &r.callPrefixes[l.frame]
			if at := slices.IndexFunc(f.undo, func(u savedVar) bool { return u.name == l.name }); at >= 0 {
				f.undo[at] = r.saveVar(l.name)
			}
			r.restoreVar(l.live)
		}
	}
}

// shellsOwnCellUnderACallPrefix is where a name's own cell is kept while an
// enclosing call's assignment prefix stands over it — the **outermost** frame
// holding it, and only when no scope is holding it further out still.
//
// Outermost because that is what the measurement says: with `t=9 outer` and
// `t=7 inner` both standing, the inner call gives back `9` and the outer one
// gives back what the declaration wrote, so the write went past the inner
// frame's entry and into the outer one's. Three prefixes deep behaves the
// same, each take-back giving back the next one out.
//
// The scope test is the other half, and it is what keeps this from taking
// over the case setGlobalVar already answers: `f(){ local q=5; q=7 g; }` has
// the scope outside the frame, so the global cell is the scope's saved copy
// and setGlobalVar writes it — measured, `q=1` at the top reads `3` after
// `f`. A scope *inside* the frame — a `local` in the callee, or one in the
// caller with the prefix further out still — leaves the frame outermost and
// this one answers. scopeDepth is what tells the two apart, the two stacks
// being pushed from the same place in the same order.
func (r *Runner) shellsOwnCellUnderACallPrefix(name string) (frame, at int, ok bool) {
	outer := -1
	for i := range r.callPrefixes {
		if slices.Contains(r.callPrefixes[i].names, name) {
			outer = i
			break
		}
	}
	if outer < 0 {
		return 0, 0, false
	}
	for i, sc := range r.scopes {
		if _, saved := sc.saved[name]; !saved {
			continue
		}
		if r.callPrefixes[outer].scopeDepth > i {
			// A scope took the name before this frame was pushed, so the
			// shell's own cell is that scope's copy and not the frame's.
			return 0, 0, false
		}
		break
	}
	at = slices.IndexFunc(r.callPrefixes[outer].undo,
		func(u savedVar) bool { return u.name == name })
	if at < 0 {
		// Held and not saved, which is a prefix this dialect keeps: there is
		// nothing underneath it to write.
		return 0, 0, false
	}
	return outer, at, true
}
