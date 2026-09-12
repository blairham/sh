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

// indirectBase is namedBase with the one extra shape a `(P)` can hand it: a
// resolved text that is a **reference** rather than a name.
//
// The resolved text is read as the parameter expansion it spells, so every
// subscript this shell answers anywhere it answers here. It used to be taken
// apart by hand into a name and one arithmetic index, and everything else
// fell through to the plain-name lookup and found nothing — silently, at
// status 0. Measured on zsh 5.9.2, 2026-09-12:
//
//	x=(p q);   v='x[@]'      p q   a whole-array subscript, was empty
//	x=(p q);   v='x[*]'      p q   likewise, was empty
//	x=(p q r); v='x[1,2]'    p q   a range, was the *second element*
//	s=abc;     v='s[2]'      b     a character of a scalar, was empty
//	a=(p q);   v='a[(r)q]'   q     a search, was empty
//	a=(p q);   v='a[(i)q]'   2     and its index form, was empty
//	x=(p q);   v='x[1]'      p     the control: one element already worked
//
// The subscript is *live* text and not a literal, which the parse is what
// makes true: measured, `i=2; v='x[$i]'` and `v='x[$(echo 2)]'` both read the
// second element, so a substitution written into a resolved reference is
// performed when the reference is read (#1852).
func (r *Runner) indirectBase(name, flags string) (words []string, set, isList bool) {
	if e, ok := r.reference(name); ok {
		return r.referenceBase(e, flags)
	}
	return r.namedBase(name, flags)
}

// lookupFlags is the part of a `(P)` group that belongs to the *second*
// lookup: `k` and `v`, and nothing else.
//
// It is baseFlags read the other way round. Those two letters are answered by
// whichever lookup the substituted value is finally taken from, baseFlags
// keeps them out of the one that produced the name, and this is the lookup
// they were being kept for — so a reference has to be read with them.
// Measured on zsh 5.9.2 with `typeset -A tab=(k1 v1 k2 v2)` and `x=(p q)`:
// `v='tab[k1]'; ${(kP)v}` is `k1` where `${(P)v}` is `v1`, `v='tab[@]';
// ${(kP)v}` is the keys, and `v='x[2]'; ${(kP)v}` is `2`, the index an
// ordinary array reads that letter as.
//
// The rest of the group is left behind because it acts on the words *below*
// this step — `(U)` cases what came out, `(j)` joins it — and handing it to
// the lookup would run it twice.
func lookupFlags(flags string) string {
	return strings.Map(func(c rune) rune {
		if c == 'k' || c == 'v' {
			return c
		}
		return -1
	}, flags)
}

// reference reads a resolved text that carries a subscript as the parameter
// expansion it spells, and reports whether it is one.
//
// False for a plain name, which is every caller's common case, and for a
// bracketed text whose head is not a name: a resolved text is a value and may
// hold anything, so `a b` and `#` reach here as readily as a name does.
func (r *Runner) reference(text string) (*syntax.ParamExpr, bool) {
	base, _, ok := r.subscriptOperand(text)
	if !ok || !isNameLike(base) {
		return nil, false
	}
	e := syntax.NewParser("", r.dialect()).ParseReference(text, syntax.Pos{})
	if e == nil || e.Bad || e.Index == nil || e.Name != base {
		// A grammar without subscripts, or a text the reader would call a
		// bad substitution. Left to the plain-name lookup, which is what it
		// got before there was a parse here at all.
		return nil, false
	}
	return e, true
}

// referenceBase is what such a reference comes to, in the three parts a base
// is: the words, whether the parameter was set, and whether it is a list.
//
// flagBase is what answers it, which is the point: the reference *is* a
// parameter expansion with a subscript, so the whole of that reading — the
// association's key, the whole-array forms, a range's list-ness, the
// set-ness of an element that is not there — comes from the one function
// that already has it rather than from a second copy that would drift.
//
// The set-ness it reports is what keeps `${(P)+v}` and `${(P)v:=w}` asking
// about the element the reference names rather than about the name that
// spelled it: measured on zsh 5.9.2, with `typeset -A M`, `x="M[k]"` and
// `M[k]=old`, `${(P)x}` is `old`, `${(P)+x}` is 1 and `${(P)x:=new}` leaves
// `old`, while with the key absent the same three are empty, 0 and `new`.
func (r *Runner) referenceBase(e *syntax.ParamExpr, flags string) (words []string, set, isList bool) {
	ref := *e
	ref.Flags = lookupFlags(flags)
	ref.HasFlags = ref.Flags != ""
	return r.flagBase(&ref)
}

// referenceKeepsFields reports whether a resolved text names the whole of an
// array with `[@]`, whose fields survive the quoted join exactly as
// `"${a[@]}"`'s do.
//
// The written `@` is what carries it and not the list-ness, which is measured
// rather than derived: on zsh 5.9.2 with `x=(p q)`, `"${(P)v}"` is two fields
// for `v='x[@]'` and one joined field for `v='x[*]'`, for `v='x[1,2]'` and
// for a `v` naming the array outright — the same three-way split `"${a[@]}"`,
// `"${a[*]}"` and `"$a"` make on a name.
//
// Asked of the text rather than of the parsed node so that nothing is
// expanded twice: a subscript holding a command substitution would run it
// here and again where the reference is read.
func (r *Runner) referenceKeepsFields(text string) bool {
	base, sub, ok := r.subscriptOperand(text)
	return ok && isNameLike(base) && sub == "@"
}

// referenceNode is the node a reading builds over a resolved text, with the
// subscript the *outer* expansion wrote — if it wrote one — reading what the
// reference named.
//
// Two callers, and both are the nested spelling of the same question:
// `${#${(P)v}}` measures the parameter the text refers to, and
// `${${(P)v}[2]}` subscripts it. Where the text carries a subscript of its
// own the two are chained, which is what makes `v='x[@]'` name the array and
// the `[2]` name an element of it rather than of nothing — measured on zsh
// 5.9.2 with `x=(p q r)`, `${#${(P)v}}` is 3 and `${${(P)v}[2]}` is `q`.
func (r *Runner) referenceNode(text string, outer *syntax.ParamExpr, src string) *syntax.ParamExpr {
	e, ok := r.reference(text)
	if !ok {
		e = &syntax.ParamExpr{Name: text}
	} else {
		ref := *e
		e = &ref
	}
	if outer != nil && outer.Index != nil {
		if ok {
			// The reference's own subscript is the link *before* the outer
			// one rather than something the outer replaces: with `v='x[@]'`,
			// `${${(P)v}[2]}` reads the second element of `x` and not the
			// second of a name that holds nothing. Overwriting it instead
			// threw the reference's half away, which is the whole of what
			// the text said.
			e.Leading = append(append([]syntax.LeadingIndex(nil), e.Leading...),
				syntax.LeadingIndex{Index: e.Index, Flags: e.IndexFlags})
		}
		e.Index, e.IndexFlags = outer.Index, outer.IndexFlags
	}
	if outer != nil {
		e.Length = outer.Length
	}
	e.Src = src
	return e
}

// assignIndirect writes the parameter a `(P)` group's base named.
//
// The element shapes go through the same two stores an ordinary `m[k]=v` and
// `a[3]=v` reach, so what they do about a missing key, an index past the end
// and the array's base is answered in one place rather than twice.
func (r *Runner) assignIndirect(written string, t *indirectTarget, v string) bool {
	name := t.name(written)
	if e, ok := r.reference(name); ok {
		return r.assignThroughReference(e, v)
	}
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

// assignThroughReference writes through a resolved text read as the parameter
// expansion it spells, which is the same node the *read* side uses.
//
// The text used to be taken apart by hand into a name and one arithmetic
// subscript, and every other shape either fell through to that reading or
// failed in the arithmetic. Measured on zsh 5.9.2, 2026-09-12, with the
// resolved text on the left:
//
//	a=(p q);   v='a[(r)q]'   `p Z`     a search names the element
//	x=(p q r); v='x[1,2]'    `Z r`     a range names a *span*
//	s=abc;     v='s[2]'      `aZc`     the control, already right
//
// The middle row is the one that was silent: `1,2` reached the arithmetic as
// one expression, the comma operator answered its right operand, and the
// write landed on element 2 instead of replacing the span — a plausible
// array back at status 0 (#2169).
//
// A whole-array subscript is *not* answered here and is refused as it was:
// `x[@]=Z` replaces the array with one element in that shell, and the direct
// spelling of it refuses too, so answering only the indirect one would put
// the two spellings out of step.
func (r *Runner) assignThroughReference(e *syntax.ParamExpr, v string) bool {
	if e.IndexFlags != nil {
		// A search names the element, on this side exactly as `a[(r)y]=Q`
		// does — and through flaggedTargetIndex, so the refusal is the fatal
		// one an assignment earns rather than the per-operand one `unset`
		// gets.
		idx, ok := r.flaggedTargetIndex(e, true)
		if !ok {
			return false
		}
		r.setArrayElem(e.Name, idx, r.subscriptText(e.Subscript()), v)
		return true
	}
	if e.IndexRange != nil {
		return r.assignReferenceSpan(e, v)
	}
	sub := r.subscriptText(e.Index)
	if r.assocDeclared(e.Name) {
		r.setAssocElem(e.Name, r.assocKey(e.Index), v)
		return true
	}
	idx, err := r.subscriptValue(sub)
	if err != nil {
		r.fatal("%s\n", r.subscriptFailure(sub, err))
		return false
	}
	r.setArrayElem(e.Name, idx, sub, v)
	return true
}

// assignReferenceSpan is a resolved text naming a **range**, where the value
// replaces the whole span rather than one element of it.
//
// The same store the direct `a[1,2]=Z` reaches, so a span that grows, shrinks
// or holds does the same thing by both spellings. Measured, `x=(p q r);
// v='x[1,2]'; ${(P)v::=Z}` leaves `Z r`.
func (r *Runner) assignReferenceSpan(e *syntax.ParamExpr, v string) bool {
	lo, ok := r.rangeEnd(e, e.IndexRange.Lo, subscriptSource{name: e.Name}, true)
	if !ok {
		return false
	}
	hi, ok := r.rangeEnd(e, e.IndexRange.Hi, subscriptSource{name: e.Name}, false)
	if !ok {
		return false
	}
	if !r.spanReplacesElements(e.Name) {
		// A dialect without the range reading has nothing to replace, and a
		// table has no span at all. Left to the ordinary element store, which
		// is what the text spells everywhere else.
		return false
	}
	elems, _ := r.arrayElemsOfTheName(e.Name)
	r.spliceElementSpan(e.Name, r.subscriptText(e.Index), elems, lo, hi, []string{v})
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
	if e.Inner == nil && e.Index != nil &&
		(r.subscriptYieldsAList(e) || r.subscriptNamesNoElementAtAll(e)) {
		words, set, _ = r.namedBase(e.Name, baseFlags(e.Flags))
	}
	return strings.Join(words, " "), set
}

// subscriptNamesNoElementAtAll is the one index that is not read as an index
// here: the one before the first, which no element has.
//
// Every other subscript naming nothing is *no name* — `${(P)n[4]}` on three
// elements is empty — and this one resolves the base's first element as
// though no subscript had been written. Measured on zsh 5.9.2, 2026-09-12
// with `n=(x y z)` and `x=(p q)`: `${(P)n[0]}` is `p q`, and so is
// `${(P)n[1-1]}`, so it is the value the expression comes to and not the
// numeral. `${(P)n[-4]}` is empty, which says it is that index and not
// "out of range below".
//
// An **association** is outside it: `[0]` is a key there like any other, and
// `${(P)nt[0]}` on a table with no such key is empty.
//
// A corner no script can depend on, and reproduced rather than left because
// the alternative is a plausible empty at status 0 (#1852).
func (r *Runner) subscriptNamesNoElementAtAll(e *syntax.ParamExpr) bool {
	if len(e.Leading) > 0 || e.IndexFlags != nil {
		return false
	}
	if _, isAssoc := r.assocFor(e.Name); isAssoc {
		return false
	}
	n, err := r.subscriptValue(r.subscriptText(e.Subscript()))
	return err == nil && n == r.arrayBase()-1
}
