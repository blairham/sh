// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// A subscript on the *left* of an assignment, where the name is already
// holding a plain string.
//
// The reading half of this rule was already answered — ScalarSubscriptIsACharacter
// is what makes `${s[2]}` a character rather than an element — and the writing
// half was not: every subscripted assignment took the array route, so a string
// was converted into an array by being written through. Measured on zsh 5.9.2
// with `buf=xy`, `buf[3]=Z` is `xyZ` and this shell gave an array of three
// elements whose first two were empty, at status 0 and with nothing said.
//
// It is a daily-driver blocker rather than a corner because `name[$#name+1]=x`
// is how a script *appends to a string* in that dialect. A prompt theme's
// worker reads its responses that way, one `sysread` at a time into
// `buf[$#buf+1]`, and a `buf` that came back as an array is not a diagnostic
// anywhere — it is a response silently misread (#1746).
//
// One axis and not two. The panel has one member that reads a subscript on a
// string as a character at all, and it writes through the same reading, so
// nothing measured disagrees about the two halves and a second field would be
// a column with one answer in it. Measured 2026-09-10 with `v=abc; v[2]=X`:
// zsh 5.9.2 leaves `aXc`, and bash 5.3, bash 3.2, bash-as-sh and ksh93 all
// leave a two-element array whose `$v` still reads `abc` — the element reading,
// which is ScalarSubscriptIsACharacter answered No. dash has no arrays and
// takes the whole word for a command name.

// subscriptSplicesCharacters reports whether a subscripted assignment to this
// name replaces characters of a string rather than reaching an element.
//
// Which names it is true of is measured rather than reasoned, and the boundary
// is *whether the name is holding a string right now*:
//
//	v=abc;      v[2]=X    aXc                       a string splices
//	v=;         v[2]=X    X                         and so does an empty one
//	typeset v;  v[2]=X    X                         a declaration is holding one
//	unset v;    v[2]=X    typeset -a v=( '' X )     nothing at all becomes an array
//	v=(x y z);  v[2]=X    the element                an array stays an array
//	typeset -A v          the key                    and so does a table
//
// The third and fourth rows are the pair that carries it: `typeset v` leaves
// the name set to the empty string — `${+v}` is 1 — and the *same* subscript
// on a name nobody declared builds an array with a gap in front of it. So the
// question is not "has this name a value" but "is there a name here", which is
// exactly the boundary spanReplacesElements already draws from the other side.
func (r *Runner) subscriptSplicesCharacters(name string) bool {
	if _, isArray := r.Arrays[name]; isArray {
		return false
	}
	if r.assocDeclared(name) {
		return false
	}
	if _, held := r.getVar(name); !held {
		return false
	}
	return r.ask(r.sem().ScalarSubscriptIsACharacter,
		"`${s[2]}` naming a character of a string")
}

// spliceScalarElem is a single subscript on the left: the span of exactly one
// character it names is replaced by the value, however many characters that is.
//
// A single subscript is a span of one and not a position to overwrite, which
// is the row a length-preserving reading gets wrong. Measured on `v=abc`:
// `v[2]=X` is `aXc`, `v[2]=XY` is `aXYc` and `v[2]=` is `ac`, so the string
// grows and shrinks with what goes in.
//
// The refusal is the array's, reached through a string: a non-negative
// subscript below the first character is `assignment to invalid subscript
// range` and ends the script at 1, exactly as `a[0]=x` does to an array.
// Which subscript that is comes from the base alone, so with `ksharrays` set
// `v[0]=X` is the first character and nothing is refused — measured, it leaves
// `Xbc`.
func (r *Runner) spliceScalarElem(name string, idx int, sub, value string, join bool) {
	if r.spanIsBelowTheFirstElement(idx, idx) {
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", name, sub))
		return
	}
	r.spliceCharacterSpan(name, idx, idx, value, join)
}

// spliceCharacterSpan replaces the characters a span names with the value.
//
// The span is spanOver's, which is the same arithmetic an array's range takes
// — one rule about what a pair of subscripts reaches, read over characters
// instead of elements — and every row of it was measured on `v=abc` rather
// than carried over on the strength of the symmetry:
//
//	v[2,3]=XY    aXY      a span replaced by as many characters as it likes
//	v[2,3]=X     aX       fewer shrinks the string
//	v[2,3]=XYZW  aXYZW    and more grows it
//	v[2,-1]=X    aX       a negative end counts back from the last character
//	v[0,1]=X     Xbc      a start below the first is the first
//	v[3,2]=X     abXc     an end before the start is an empty span *at* the
//	                      start, so the value goes in and nothing comes out
//	v[1,0]=X     Xabc     including in front of everything
//	v[0,0]=X     refused  a span wholly below the first character
//
// Past the last character the value is **appended and the gap is not padded**,
// which is the row an array does differently and the reason this is not
// spliceElementSpan over one string: `v[4]=X` and `v[10]=X` are both `abcX`,
// where `a=(x y z); a[5]=Q` pads with an empty element. A string has no
// positions to hold empty, so there is nothing to pad with — and that is what
// makes `buf[$#buf+1]=x` an append rather than an append with a hole in front
// of it the first time the script runs one subscript long.
//
// join is `+=`, which reads the span it names and puts the value after it:
// `v=abc; v[2]+=X` is `abXc` — `b` joined to `X` back where `b` was — and
// `v[2,3]+=X` is `abcX`. Not appendedValue's join, because a numeric name
// never arrives here.
func (r *Runner) spliceCharacterSpan(name string, from, to int, value string, join bool) {
	if r.numericSlice(name, value, join) {
		return
	}
	v, _ := r.getVar(name)
	chars := r.units(v)
	first, tail, within := r.spanOver(len(chars), from, to)
	if !within {
		// The span begins past the last character. Nothing comes out and
		// nothing is padded — see above.
		r.setVar(name, v+value)
		return
	}
	if join {
		value = strings.Join(chars[first:tail], "") + value
	}
	r.setVar(name, strings.Join(chars[:first], "")+value+strings.Join(chars[tail:], ""))
}

// numericSlice is what a subscript does to a name carrying an arithmetic
// attribute, and reports whether it has taken the write.
//
// Such a name has no characters for a subscript to reach: its value is a
// number that a spelling happens to print. Measured 2026-09-10 on zsh 5.9.2,
// where the subscript is *ignored* and the value replaces the whole name —
// with `typeset -i n=123`, `n[2]=9` and `n[5]=9` and `n[2,3]=99` are `9`, `9`
// and `99`, and not the `193`, `1239` and `199` a splice would leave. The
// float attribute answers alike: `typeset -F f=1.5; f[2]=9` is
// `9.0000000000`.
//
// `+=` is refused outright rather than ignored, which is the half that would
// have been guessed wrong: `typeset -i n=12; n[1]+=9` is `attempt to add to
// slice of a numeric variable` and ends the script at 1, and so is the float
// and the range. Adding *to a slice* is what has no meaning here — the
// unsubscripted `n+=9` is ordinary arithmetic and is not this.
func (r *Runner) numericSlice(name, value string, join bool) bool {
	_, isFloat := r.floatPrecision[name]
	if !isFloat && !r.integer[name] {
		return false
	}
	if join {
		r.fatal("%s\n", Wording(r.diag().AppendToANumericSlice,
			"%[1]s: cannot append to a slice of a numeric variable", name))
		return true
	}
	// The subscript is ignored and the value lands whole, where the store
	// folds it through the attribute exactly as an unsubscripted assignment
	// would: `typeset -i n=12; n[9]=abc` is `0`, which is the arithmetic
	// reading of `abc` and not a character anywhere.
	r.setVar(name, value)
	return true
}
