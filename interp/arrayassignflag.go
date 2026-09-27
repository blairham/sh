// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(A)` expansion flag over an **assignment**: `${(A)name=value}` stores
// the words the value comes to as an *array* where the plain assignment would
// store one string.
//
// One grammar in the panel has the flag, so what it means here is that
// shell's answer, measured 2026-09-26 on zsh 5.9.2 under `-f` and recorded in
// docs/spec/grammar/parameter-expansion.md. Every row below is from that run.
//
// The flag does nothing at all where the expansion does not assign — see
// expandflags.go, where the six lines the panel's plugin managers actually
// write are all of that kind — and this file is only the other half.
//
//	unset u; ${(A)u=x y}     typeset -a u=( 'x y' )   one element
//	unset u; ${(A)=u=x y}    typeset -a u=( x y )     two, the `=` splitting
//	unset u; ${(As:,:)u=a,b} typeset -a u=( a b )     and the group's own split
//	u=set;   ${(A)u=x y}     u is still the scalar `set`, `=` not having fired
//
// **The array is the value's own fields and not the pipeline's.** The rest of
// the group runs on what the expansion *yields* and never reaches the store:
// `${(AU)u=x y}` leaves `x y` in lower case, `${(AO)=u=b a c}` leaves
// `b a c` unsorted, and `${(Aj:-:)u=$a}` with `a=(1 2)` leaves two elements
// rather than one joined. So this runs where the assignment fires and takes
// the fields as the operand produced them.
//
// **Splitting is the one part of the group that does reach it**, because
// splitting is what makes fields out of a value in the first place — and the
// two spellings of it are not the same rule, which is measured:
//
//	${(A)=u="x y"}     one element, the quotes protecting the space
//	${(As:,:)u="a,b"}  two, the `s` separator splitting the text regardless
//
// so the `=` is the ordinary field split with the ordinary quoting rule, and
// `(s)`, `(f)` and `(0)` are a split of the finished text.
//
// **One row of the *yield* is not carried, and it is named here rather than
// left to be found.** Where an `(A)` assignment fires under an `=` and the
// whole expansion is written inside quotes, that shell does not split the
// result: `print -rl -- "${(A)=u=x y}"` is the one line `x y`, where this
// splits it into two fields. The `=` splitting a quoted result is the flag's
// ordinary rule — `"${=v}"` on `a b` is two fields in both shells — and the
// suspension is specific to the assignment having fired, which this pass
// cannot see from where the split is decided without re-deriving the
// operator's test. The **store** is right in that shape, which is what the
// flag is for; the fields the expansion yields are one too many.

// arrayAssignFlag reports whether this expansion's group turns its assignment
// into an array assignment.
//
// The doubled letter is deliberately not here. `(AA)` makes the name an
// *association*, which needs the value's fields paired off and a refusal of
// its own for an odd count — a separate measurement, and one this does not
// carry. It is still refused by name in flaggedWords, so a script asking for
// a table is told rather than handed an indexed array (#4453).
func arrayAssignFlag(e *syntax.ParamExpr) bool {
	return strings.Count(e.Flags, "A") == 1
}

// assocAssignFlag is the doubled letter, which is the one still refused.
func assocAssignFlag(e *syntax.ParamExpr) bool {
	return strings.Count(e.Flags, "A") > 1
}

// assignedArrayElements is the element list an `(A)` assignment stores: the
// fields the operand word produces, under whatever split the group asked for.
func (r *Runner) assignedArrayElements(e *syntax.ParamExpr, quoted bool) []string {
	if e.Arg == nil {
		// `${(A)u=}` has no word at all and leaves one empty element behind,
		// measured — not the empty array, which is what a missing word would
		// otherwise come to.
		return []string{""}
	}
	if strings.ContainsAny(e.Flags, splitFlagLetters) {
		// `(s)`, `(f)` and `(0)`: the finished text, split on the separator
		// the group named. Quoting does not protect here — `${(As:,:)u="a,b"}`
		// is two elements — so the text is what is split and the one splitter
		// in this package is what splits it.
		return r.splitFlagged(r.substitutedWordText(e.Arg), e)
	}
	defer r.splittingTheAssignedWord(e.SplitFlags%2 == 1, quoted)()
	return unescapeAll(r.expandWordEscaped(e.Arg))
}

// splittingTheAssignedWord arms the operand of an `(A)` assignment for field
// splitting where the group's `=` asked for it, and returns the disarm.
//
// Two things are armed and they are two different readings of the same
// question, which is why one of them is not enough. The literal text of the
// word splits — `${(A)=u=x y}` is written as one literal and is two elements
// — and so does the result of an expansion inside it, which the dialect this
// flag belongs to does *not* split of its own accord: `s="p q";
// ${(A)=u=$s}` is two elements where `${(A)u=$s}` is one. Arming only the
// literals would have got the first row right and the second wrong, and the
// first row is the one in every example.
//
// Quoting still protects, in both halves and without anything here saying so:
// `${(A)=u="x y"}` and `${(A)=u="$s"}` are each one element, because a quoted
// span is not split by either reading.
//
// The axis is *overridden* rather than a splitter of its own being called,
// which is the rule interp/splitflag.go states for the same flag's ordinary
// reading: there is one field splitter in this package, and a second would be
// a second set of edge cases to keep in step.
func (r *Runner) splittingTheAssignedWord(split, quoted bool) func() {
	if !split {
		return func() {}
	}
	savedSem, savedLit := r.Semantics, r.splitWordLiterals
	r.swapSemantics(func(s *Semantics) { s.SplitParamExpansion = Yes })
	r.splitWordLiterals = splitLiterals{on: true, answer: Yes, evenQuoted: quoted}
	return func() { r.Semantics, r.splitWordLiterals = savedSem, savedLit }
}
