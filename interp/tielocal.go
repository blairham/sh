// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// What a *local* declaration of one half of a tie means.
//
// A tie is two names for one value — see tiedscalar.go — so "declare one of
// them local" has an answer that the two names have to agree on. There are
// two answers, and which one a tie gets is decided by who made it.
//
// Measured 2026-09-09 against zsh 5.9.2 under `-f`, the one shell in the
// panel with the letter, with `PATH=/bin:/usr/bin` and a script tie
// `typeset -T SCA sca; SCA=a:b:c`. `${(t)name}` is the instrument that names
// what each declaration produced, and `$#array` is the one that says whether
// the caller's elements are still in view.
//
// The shell's **own** pairs — `PATH`/`path` and the seven beside it — shadow
// as a pair and stay tied:
//
//	f(){ local    PATH=/x; … }  → ${(t)PATH} scalar-local-tied-special
//	                              ${(t)path} array-tied-special, `path` is /x
//	f(){ local    path;    … }  → ${(t)path} array-local-tied-special
//	                              $#path 0, $PATH empty
//	f(){ local -a path;    … }  → $#path 0, $PATH empty
//	f(){ local    PATH;    … }  → $PATH empty, $#path 1, $path[1] empty
//	after any of them            → the caller's PATH *and* path are back
//
// A tie a **script** made with `typeset -T` does not shadow as a pair at all.
// A `local` of either half produces an ordinary, untied local — of whatever
// kind the declaration itself named — and leaves the other half naming the
// outer cell:
//
//	f(){ local    SCA=x;   … }  → ${(t)SCA} scalar-local, $#sca 3, sca a,b,c
//	f(){ local    sca;     … }  → ${(t)sca} scalar-local, $SCA still a:b:c
//	f(){ local -a sca;     … }  → ${(t)sca} array-local,  $SCA still a:b:c
//	f(){ local +h SCA=x;   … }  → ${(t)SCA} scalar-local — `+h` does not
//	                              tie it back, so this is not the `-h`
//	                              attribute wearing another name
//
// Three facts are in those rows.
//
//   - **The pair-shadow belongs to the shell's own ties, not to ties.** The
//     built-in halves are *special* parameters: the tie is a property of the
//     name, so a local of one is still that name and the partner has to be
//     saved with it. A script's tie is a property of the parameter, and
//     `local` makes a new parameter, which is simply not tied.
//
//   - **So a script tie goes out of effect inside the local and comes back
//     after it**, which is the same suspension `-h` asks for and reaches it
//     through the same predicate, tieDetached. It is *not* the same letter,
//     though: row `local +h SCA=x` is the control that says so, since `+h`
//     takes the attribute off and the tie stays gone.
//
//   - **Only a scope deeper than the tie's own detaches it.** `typeset -T`
//     inside a function shadows both halves itself, and that declaration's
//     own shadow must not turn the tie it is making off — measured,
//     `f(){ typeset -T LOC loc; LOC=a:b; }` leaves `$#loc` 2 inside `f`. So
//     the tie records the depth it was made at and only the scopes past it
//     are asked. A nested function's `local LOC` does detach it.
//
// The `local` and the `typeset` spellings are one question here, so this
// hangs off shadow, which is the single place a declaration saves a name.

// shadowTiedHalf saves the *other* half of a tie in the same scope, so a
// declaration of one built-in half is a declaration of the pair.
//
// Reached from shadow itself rather than from the three declaration loops,
// because the rule is about saving a name and that is the one place every
// spelling of a declaration goes through.
//
// The recursion terminates on the entry shadow has already written: the
// partner's own call finds this name saved and stops. Guarding on that rather
// than a flag keeps a second declaration of the same name in the same scope
// from re-saving the partner over the local it now holds.
func (r *Runner) shadowTiedHalf(name string) {
	t, ok := r.tieOf(name)
	if !ok || !t.special {
		return
	}
	other := t.array
	if name == t.array {
		other = t.scalar
	}
	if _, seen := r.scopes[len(r.scopes)-1].saved[other]; seen {
		return
	}
	r.shadow(other)
}

// tieDetached reports whether a tie is out of effect right now, which is the
// one predicate both answers above come out of.
//
// A tie no local stands over is always in effect, whatever attributes its
// names carry — `typeset -h PATH; PATH=/x` at the top level moves `path` with
// it, measured. Past that the two answers part: a shadow of a script's tie is
// a new parameter that is simply not tied, and a shadow of one of the shell's
// own halves is still that special parameter unless `-h` says otherwise.
func (r *Runner) tieDetached(t tie) bool {
	if !r.tieShadowedInItsScope(t) {
		return false
	}
	return !t.special || r.hidesItsTie(t)
}

// tieShadowedInItsScope reports whether a scope the tie is *subject to* has a
// half of it displaced — one deeper than the scope the tie was made in.
//
// The depth is what keeps a tie's own declaration from turning it off.
// `typeset -T` inside a function shadows both halves before recording the
// tie, so asking "is either half shadowed anywhere" answered yes for every
// function-local tie there is: measured, `f(){ typeset -T S s; typeset -h S;
// S=one:two; }` leaves `$s` holding `one two` in the shell with the letter,
// where the broader question detached it and left `$s` empty.
func (r *Runner) tieShadowedInItsScope(t tie) bool {
	if t.depth >= len(r.scopes) {
		return false
	}
	for _, sc := range r.scopes[t.depth:] {
		if _, saved := sc.saved[t.scalar]; saved {
			return true
		}
		if _, saved := sc.saved[t.array]; saved {
			return true
		}
	}
	return false
}

// declaredEmptyTieArray is what a valueless declaration of the *array* half of
// a live tie does, and it is a branch of declareEmpty rather than a rule of
// its own.
//
// The half decides the kind. Declaring the scalar half empty sets a string,
// and the mirror splits that string into the one field it has — measured,
// `local PATH` leaves `$#path` 1 with an empty element. Declaring the array
// half empty sets *no* elements, and the mirror joins nothing into the empty
// string — `local -a path` leaves `$#path` 0 and `$PATH` empty. Setting the
// array half's cell to an empty scalar answered the second with the first and
// left the caller's elements in view besides, since the elements live in a
// table of their own.
//
// Without `-a` too: the array half of a built-in pair is an array whatever
// the declaration says, because the parameter's kind is the shell's and not
// the line's — `local path` reads back as `array-local-tied-special`.
func (r *Runner) declaredEmptyTieArray(name string) bool {
	t, ok := r.tieOf(name)
	if !ok || t.array != name || r.tieDetached(t) {
		return false
	}
	r.setArray(name, nil)
	return true
}
