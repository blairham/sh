// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// How a produced parameter lists back.
//
// A produced parameter is in none of the tables a listing walks — not Vars,
// not Arrays, not the keyed one — so `typeset -p RANDOM` answered
// `RANDOM: not found` at 1 from a name the same shell had just expanded a
// number for: a listing and an expansion giving two answers to whether a name
// exists (#2451). Every shell in the panel that has the parameter lists it,
// with its value.
//
// Until this, the only thing that could put one into a listing was
// [Runner.MarkReadonly] — the `attributed` guard in declarationOf reaches a
// producer only through an attribute — so a listing was working for
// `zsh/datetime`'s three, which happen to be readonly, and for nothing else.
// A parameter a script may assign to must not be marked readonly, so the mark
// could not be borrowed for the rest; and the attribute it stands for is not
// the attribute a listing writes anyway.
//
// Two facts have to be stated rather than derived, and both are per dialect,
// which is why they are a value the *registering* dialect supplies:
//
//  1. **Which letters.** bash 5.3 writes `declare -i RANDOM="16735"` and
//     `declare -- LINENO="1"`; bash 3.2 writes `-i` for both; ksh93 writes
//     `typeset -i RANDOM=7000`; zsh writes `typeset -i10 RANDOM=13859`. A
//     produced parameter carries no attribute record here to read those off.
//  2. **Which names a listing names at all.** zsh writes *nothing* for
//     `typeset -p LINENO`, at status 0 — neither a listing nor a refusal,
//     which is a third answer rather than a variant of either. Silent says
//     that, and a produced parameter no dialect has registered here keeps the
//     answer it had: not there.
//
// Measured 2026-09-12, `env -i PATH=/usr/bin:/bin` with a scratch HOME, over
// a script file, `typeset -p NAME`:
//
//	           RANDOM                    SECONDS                LINENO
//	bash 5.3   declare -i RANDOM="…"     declare -i SECONDS="0" declare -- LINENO="1"
//	bash 3.2   declare -i RANDOM="…"     declare -i SECONDS="0" declare -i LINENO="1"
//	ksh93u+    typeset -i RANDOM=7000    typeset -F 3 SECONDS=… typeset -i LINENO=1
//	zsh 5.9.2  typeset -i10 RANDOM=…     typeset -i10 SECONDS=0 nothing, status 0
type ProducedDeclaration struct {
	// Integer is the `-i` letter.
	Integer bool
	// Base is the output base written on that letter — zsh's `-i10`. Zero
	// where the dialect writes no base, which is every shell but that one.
	Base int
	// Float is the `-F` letter. The *places* beside it are ksh93's
	// `typeset -F 3` and are not written by any listing form here yet
	// (#1461), which is why ksh93's `SECONDS` is deliberately not registered:
	// `typeset -F SECONDS=0.001` would be closer than `not found` and still
	// not right, and a corpus row cannot tell "closer" from "right".
	Float bool
	// Silent is zsh's answer for `LINENO`: the name is known to a listing,
	// which writes nothing for it and reports 0. Without it the choice is
	// between a row no shell writes and the `not found` this issue is about.
	Silent bool
}

// SetDynamicDeclaration says how a produced parameter lists back.
//
// It travels beside [Runner.SetDynamic], which is the seam a dialect
// registers the producer at, and is required for a produced parameter a
// listing should name — a producer with no declaration is not listed, which
// is the answer every parameter here had before this and the right one for a
// name the shell being modeled does not list either.
//
// Deliberately not the attribute tables. Marking `RANDOM` integer would put
// the letter in a listing by the route an ordinary name takes, and would also
// change what `RANDOM=abc` does, what `typeset +i RANDOM` can take off, and
// what an `unset` clears — three behaviors nobody measured, riding on a
// decision about a listing. This states the listing and nothing else.
func (r *Runner) SetDynamicDeclaration(name string, d ProducedDeclaration) {
	if r.dynamicDeclarations == nil {
		r.dynamicDeclarations = map[string]ProducedDeclaration{}
	}
	r.dynamicDeclarations[name] = d
}

// producedDeclaration is what a listing was told about this name, and whether
// it was told anything.
//
// A name `unset` has taken away is not listed however it was registered: the
// parameter is gone until something brings it back, and a listing that
// reached the producer anyway would draw a value from a name a read of which
// answers nothing.
func (r *Runner) producedDeclaration(name string) (ProducedDeclaration, bool) {
	if r.removed[name] {
		return ProducedDeclaration{}, false
	}
	d, ok := r.dynamicDeclarations[name]
	return d, ok
}
