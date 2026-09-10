// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// The `(P)` flag beside an operator that assigns.
//
// `(P)` says the base is the *name* of the parameter the expansion is about,
// and an assignment written in the same expansion lands on that parameter
// rather than on the name that spelled it. Measured on zsh 5.9.2 —
// the only shell in the panel with the flag — on 2026-09-10:
//
//	x=tgt; tgt=old; ${(P)x::=new}   new   and leaves tgt=new, x=tgt
//	x=tgt; unset tgt; ${(P)x:=new}  new   and leaves tgt=new
//	x=tgt; tgt=old;  ${(P)x:=new}   old   the test is about tgt, not x
//	x=tgt; tgt=;     ${(P)x=new}          `=` fires on unset alone
//	x=tgt; tgt=old; ${(UP)x::=new}  NEW   and leaves tgt=new, so the other
//	                                      letters still act on what is
//	                                      substituted and not on what is
//	                                      stored
//
// Reading the flag only on the way *out* — which is what this shell did —
// assigned to the name instead: `${(P)x::=new}` left `x=new` and `tgt`
// untouched, at status 0. That is the plausible-answer failure, and it had
// a second half. The startup this was found in writes `${(P)2::=$mtime}`
// from a function whose `$2` is the name to write, so the assignment created
// a parameter *called* `2`; every later `$2` in a function called with one
// argument then read it, a plugin manager's "were two components given?"
// test answered yes, and 18 autoloaded functions were looked for in a
// directory assembled out of the wrong halves (#1672).
//
// **The base being unset is not the empty name.** Measured: `unset x;
// ${(P)x::=new}` leaves `typeset x=new`, so with nothing to resolve the
// assignment lands on the name as written, where `x=; ${(P)x::=new}` is
// `not an identifier: ` and ends the script. So the question is whether the
// base was *set*, which is what indirectTarget carries.

// indirectTarget is what a `(P)` group resolved its base to, kept from the
// step that resolved it so that an assignment further down the pipeline can
// name the same parameter without expanding the base a second time — which
// would run a command substitution in it twice.
type indirectTarget struct {
	// text is the resolved base, joined as the indirection read it.
	text string
	// set says whether the base held anything at all. An unset base leaves
	// the assignment on the name as written; see above.
	set bool
}

// name is the parameter an assignment through the group writes.
func (t *indirectTarget) name(written string) string {
	if !t.set {
		return written
	}
	return t.text
}

// indirectElement splits a resolved name that names one element of an array
// or association — `ZI[mtime-side]`, which is the shape the startup writes.
//
// ok is false for a plain name, which is every other caller's case, and for
// a bracketed text whose head is not a name: the resolved text is a value
// and may hold anything, so `a b` and `#` reach here as readily as a name
// does and are the assignment's own refusal to make rather than this split's.
func (r *Runner) indirectElement(name string) (base, sub string, ok bool) {
	base, sub, ok = r.subscriptOperand(name)
	if !ok || !isNameLike(base) {
		return "", "", false
	}
	return base, sub, true
}

// indirectElementValue reads the element a resolved name points at, so that
// the `:=` and `=` tests ask about the parameter the assignment would write.
//
// Measured on zsh 5.9.2: with `typeset -A M`, `x="M[k]"` and `M[k]=old`,
// `${(P)x}` is `old`, `${(P)+x}` is 1 and `${(P)x:=new}` leaves `old`; with
// the key absent the same three are empty, 0 and `new`. The array spelling
// answers alike — `a=(p q); x="a[2]"` reads `q`.
//
// handled is false where the head names neither an association nor an array.
// zsh reads a subscript on a scalar as a character and a subscript flag as a
// search, and this shell answers neither yet; falling through leaves those
// texts to the plain-name lookup they already got rather than putting a
// second answer in front of it.
func (r *Runner) indirectElementValue(base, sub string) (value string, set, handled bool) {
	if a, ok := r.assocFor(base); ok {
		v, held := a[sub]
		return v, held, true
	}
	arr, ok := r.Arrays[base]
	if !ok {
		return "", false, false
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		// The subscript is not arithmetic — a flag group or a range. Left to
		// the fall-through above rather than reported, because a *read* of
		// one is not this change's question and a diagnostic here would fire
		// on a line the shell answers.
		return "", false, false
	}
	pos, within := r.elemPos(arr, idx)
	if !within {
		return "", false, true
	}
	v, held := arr[pos]
	return v, held, true
}

// indirectBase is namedBase with the one extra shape a `(P)` can hand it: a
// resolved text that names an element rather than a whole parameter.
func (r *Runner) indirectBase(name, flags string) (words []string, set, isList bool) {
	if base, sub, ok := r.indirectElement(name); ok {
		if v, held, handled := r.indirectElementValue(base, sub); handled {
			return []string{v}, held, false
		}
	}
	return r.namedBase(name, flags)
}

// assignIndirect writes the parameter a `(P)` group's base named.
//
// The element shapes go through the same two stores an ordinary `m[k]=v` and
// `a[3]=v` reach, so what they do about a missing key, an index past the end
// and the array's base is answered in one place rather than twice.
func (r *Runner) assignIndirect(written string, t *indirectTarget, v string) bool {
	name := t.name(written)
	if base, sub, ok := r.indirectElement(name); ok {
		if r.assocDeclared(base) {
			r.setAssocElem(base, sub, v)
			return true
		}
		idx, err := r.subscriptValue(sub)
		if err != nil {
			r.fatal("%s\n", r.subscriptFailure(sub, err))
			return false
		}
		r.setArrayElem(base, idx, sub, v)
		return true
	}
	// Not an element, so it is a name — and the check is the one every other
	// route through the expander uses. Asked of all three operators rather
	// than of `::=` alone, which is measured: `x=; ${(P)x:=new}` and
	// `x=; ${(P)x=new}` are both `not an identifier: ` and both end the
	// script, where the *direct* `${v:=w}` has a narrower rule the panel
	// does not agree on (see assignableTarget). Nothing is guessed at by
	// asking here — the flag exists in one shell, and that shell refuses.
	if !r.assignableParamName(name) {
		return false
	}
	r.setVar(name, v)
	return true
}
