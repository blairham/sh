// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "github.com/blairham/sh/syntax"

// assignElemLiteral is `a[i]=(p q)`: an array literal standing where one
// element's value goes.
//
// It is not the whole-array assignment with a subscript stuck on the front,
// and reading it as one is the bug this exists to close — the subscript was
// dropped and the literal replaced everything the name held, silently, at
// status 0 and with an array on the other side. `fpath[$index]=()` is the line
// that made it worth finding: a plugin removing one entry from `fpath` emptied
// it instead.
//
// Which of the three readings applies is a semantics axis rather than a branch
// here, because the panel does not merely differ in degree: one shell splices,
// one refuses the line outright, and the third builds a nested value. See
// SubscriptedArrayLiteralPolicy.
func (r *Runner) assignElemLiteral(a *syntax.Assign) {
	switch r.subscriptedArrayLiteral() {
	case SubscriptedArrayLiteralRefused:
		// The subscript as written rather than as evaluated: the refusing
		// shell quotes `a[$i]` back, and it refuses before evaluating it at
		// all — `a[1/0]=(p q)` is the same sentence there and not a division
		// by zero, so nothing is asked of the subscript on this route.
		r.fatal("%s\n", Wording(r.diag().ArrayLiteralThroughASubscript,
			"%[1]s[%[2]s]: cannot assign list to array member",
			a.Name, subscriptSubject(a.IndexText, r.subscriptAsWritten(a.Index))))
	case SubscriptedArrayLiteralSplices:
		r.spliceElemLiteral(a)
	}
	// Unspecified has already been reported by name, and r.unspecified is set
	// so nothing after it writes.
}

// spliceElemLiteral replaces the element a subscript names with the words a
// literal builds, which changes the array's length by the literal's count less
// one.
//
// Measured on zsh 5.9.2, which is the only panel member that does this:
//
//	a=(x y);   a[1]=(p q)   -> p q y
//	a=(x y z); a[1]=()      -> y z
//	a=(x y);   a[1]+=(p)    -> x p y
//
// So `+=` appends to the *element* and not to the array, which is the same
// distinction a scalar `a[0]+=Q` already draws against `a+=(Q)` — the
// subscript is what says which of the two the operator means.
func (r *Runner) spliceElemLiteral(a *syntax.Assign) {
	if !r.spliceTargetIsAnArray(a) {
		return
	}
	text := r.joinWord(a.Index)
	// The subscript as written is what a boundary refusal quotes back, and it
	// is not the text the arithmetic reads — see subscriptSubject (#1373).
	subject := subscriptSubject(a.IndexText, text)
	from, to, outcome := r.assignSpan(a, text)
	if outcome == spanReported {
		return
	}
	if outcome == spanResolved {
		// A *range* of elements rather than one: the words replace the whole
		// span, so `a=(1 2 3); a[2,3]=(x y)` is three elements and not four.
		// See spliceElementSpan for what each end is measured to do.
		words, ok := r.literalWords(a.Name, a.Elems)
		if !ok {
			return
		}
		elems, _ := r.arrayElemsOfTheName(a.Name)
		r.spliceElementSpan(a.Name, subject, elems, from, to, words)
		return
	}
	idx, err := r.subscriptValueAsWritten(subject, text)
	if err != nil {
		// The same failure, worded the same way, as the scalar element
		// assignment beside it: `a[1/0]=(p q)` is `division by zero` and ends
		// the script rather than reporting something about the array.
		r.fatal("%s\n", r.subscriptFailure(text, err))
		return
	}
	words, ok := r.literalWords(a.Name, a.Elems)
	if !ok {
		return
	}

	elems, _ := r.arrayElemsOfTheName(a.Name)
	pos, within := r.elemPos(r.Arrays[a.Name], idx)
	if !within {
		if idx < 0 && r.ask(r.sem().NegativeSubscriptPastTheStartInserts,
			"a negative subscript past the first element placing one in front of it") {
			// Past the start, so there is no element to replace and the words
			// go in front of every element there is — the same answer the
			// scalar spelling gives, reached by the same axis.
			r.setArray(a.Name, append(append([]string{}, words...), elems...))
			return
		}
		if r.unspecified {
			return
		}
		r.fatal("%s\n", Wording(r.diag().BadArraySubscript,
			"%[1]s[%[2]s]: bad array subscript", a.Name, subject))
		return
	}
	// A subscript past the last element has to *become* one before it can be
	// replaced, and what it becomes is empty: `a=(x y); a[5]=(p q)` reads back
	// six elements, the two in the middle being the padding. The same padding
	// is what an append past the end appends to, which is why it happens
	// before the operator is asked about — `a=(x y); a[5]+=(p)` leaves the
	// empty fifth element and puts p after it.
	for len(elems) <= pos {
		elems = append(elems, "")
	}
	head, tail := elems[:pos], elems[pos+1:]
	if a.Append {
		head = elems[:pos+1]
	}
	out := make([]string, 0, len(head)+len(words)+len(tail))
	out = append(out, head...)
	out = append(out, words...)
	out = append(out, tail...)
	r.setArray(a.Name, out)
}

// spliceTargetIsAnArray reports whether the name a subscripted literal writes
// through can take one, and reports it by name where it cannot.
//
// Two refusals rather than one, because the splicing shell words them apart
// and the reasons are genuinely different: a declared table has keys rather
// than positions, so there is no span for the words to replace, and a plain
// string is not an array at all. An *unset* name is neither — it becomes an
// array, with the padding in front of the subscript.
func (r *Runner) spliceTargetIsAnArray(a *syntax.Assign) bool {
	name := a.Name
	if r.assocDeclared(name) {
		r.fatal("%s\n", Wording(r.diag().SliceOfAnAssociativeArray,
			"%[1]s: attempt to set slice of associative array", name))
		return false
	}
	if _, isArray := r.Arrays[name]; isArray {
		return true
	}
	if a.Operand && r.declaredEmpty[name] {
		// A declaration's own operand — `typeset a[2]=(p q)`. The utility
		// reaches the name first and is handed it bare, so a name that had
		// nothing is holding the declaration's empty string by the time the
		// literal lands. That is this line's own doing and not a string the
		// script put there: `typeset b[1]=(p q)` splices into a fresh name,
		// while `s=abc; typeset s[1]=(p q)` is refused, and the two differ
		// only in whether the declaration had a value to leave alone.
		return true
	}
	if _, held := r.getVar(name); held {
		r.fatal("%s\n", Wording(r.diag().ArrayValueToNonArray,
			"%[1]s: attempt to assign array value to non-array", name))
		return false
	}
	return true
}

// literalWords is the words a literal builds, in order, as the words that
// would have been the whole array had the subscript not been there.
//
// Through the same placement the whole-array spelling uses, rather than by
// expanding the elements and taking the fields: a literal may place its own
// elements — `a[2]=([3]=p)` splices three positions, two of them empty — and
// two spellings of one construct must not come to disagree about that.
func (r *Runner) literalWords(name string, elems []*syntax.Word) ([]string, bool) {
	parsed, ok := r.literalElems(elems)
	if !ok {
		// A failed element list costs the splice, exactly as it costs the
		// whole-array spelling — see literalElems.
		return nil, false
	}
	built, ok := r.literalInto(name, Array{}, 0, parsed)
	if !ok {
		return nil, false
	}
	return r.readArray(built), true
}

// arrayElemsOfTheName is the elements a splice starts from: the array as the
// dialect reads it, and nothing at all for a name that holds nothing.
//
// Not arrayElems, which answers a plain scalar as a one-element array — that
// is right for `${x[0]}` and wrong here, where a scalar has already been
// refused and an unset name must start empty rather than as one empty string.
func (r *Runner) arrayElemsOfTheName(name string) ([]string, bool) {
	a, ok := r.Arrays[name]
	if !ok {
		return nil, false
	}
	return r.readArray(a), true
}
