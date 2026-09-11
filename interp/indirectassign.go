// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

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

// indirectSpecialNames are the one-character parameter names a `(P)` can read
// out of the front of its base. A name of letters, of digits, or one of
// these, and nothing else: measured on zsh 5.9.2 with `set -- aa bb cc`,
// `x=(p q)` and `_u=UU`, the text on the left resolving to the value on the
// right.
//
//	'x junk'   p q       the name is read off the front and the rest dropped
//	'x-y'      p q       any character a name cannot hold ends it
//	'x=y'      p q
//	'x.y'      p q
//	'_u-z'     UU        `_` starts one
//	'2x'       bb        a digit run is a positional parameter
//	'12x'      ``        and `$12` is nothing with three of them
//	'#x'       3         a one-character special name is that one character
//	'@x'       aa bb cc
//	' x'       ``        so a leading space is *no* name at all
//	'x[1] junk' p        a subscript is part of the name
const indirectSpecialNames = "@*#?-$!"

// indirectName is the parameter name a `(P)` reads out of the text it
// resolved its base to.
//
// **The text is read from the front and the rest is discarded**, which is the
// whole of this function and is measured rather than assumed — see the table
// above. Looking the *whole* text up instead found nothing whenever the base
// held more than one word, so `${(P)two}` over a two-element array and
// `${(P)n2}` over a scalar holding `x y` both substituted empty at status 0
// (#1639, #1543). Empty is also the answer where the front is not a name at
// all, and that is a measurement too: a leading space is not skipped.
//
// A base holding several words is *joined* before it gets here, which is what
// makes the array and the scalar one rule rather than two — the first element
// is what the front of the join comes from either way.
func indirectName(text string) string {
	if text == "" {
		return ""
	}
	switch c := text[0]; {
	case c == '_' || isLetter(c):
		i := 1
		for i < len(text) && (text[i] == '_' || isLetter(text[i]) || isDigit(text[i])) {
			i++
		}
		if i < len(text) && text[i] == '[' {
			n := subscriptSpan(text[i:])
			if n == 0 {
				return ""
			}
			return text[:i+n]
		}
		return text[:i]
	case isDigit(c):
		i := 1
		for i < len(text) && isDigit(text[i]) {
			i++
		}
		return text[:i]
	case strings.IndexByte(indirectSpecialNames, c) >= 0:
		return text[:1]
	}
	return ""
}

// subscriptSpan is the length of the bracketed subscript at the front of s,
// counting nested brackets so that `a[b[1]]` is not cut at the first close.
//
// Zero for an unterminated bracket, which leaves the name empty rather than
// dropping the bracket and answering with the whole parameter: zsh reports
// `invalid subscript` for `x=(p q); v='x['; ${(P)v}`, and the elements would
// be a plausible answer at status 0 where the shell refuses the line.
func subscriptSpan(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			if depth--; depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}

// indirectSourceText is the text a `(P)` resolves its base to: the base's
// words joined, with one correction that a subscript makes.
//
// **A subscript that names a list is not part of the resolution.** Measured
// on zsh 5.9.2 with `x=(p q); y=(r s); z=(t u); n=(x y z)`:
//
//	${(P)n}        p q   with no subscript the first element is the name
//	${(P)n[@]}     p q   and `[@]` changes nothing
//	${(P)n[*]}     p q
//	${(P)n[1,2]}   p q   nor does a range — not even one that starts past
//	${(P)n[2,3]}   p q   the first element, or names a single one
//	${(P)n[3,3]}   p q
//	${(P)n[5,6]}   p q   or names none at all
//	${(P)n[2]}     r s   while an index naming one element *is* the name
//	${(P)n[-1]}    t u
//	${(P)n[4]}     ``    and one naming nothing is no name
//
// So the two readings split on the subscript's *shape* and not on what it
// selected, which is subscriptYieldsAList's question and is asked nowhere
// else. Taking the selected elements instead answered `r s` for the two rows
// that start at the second element — a plausible value at status 0.
func (r *Runner) indirectSourceText(e *syntax.ParamExpr, words []string, set bool) (string, bool) {
	if e.Inner == nil && e.Index != nil && r.subscriptYieldsAList(e) {
		words, set, _ = r.namedBase(e.Name, baseFlags(e.Flags))
	}
	return strings.Join(words, " "), set
}
