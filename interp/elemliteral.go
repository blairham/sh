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
// one refuses the line outright, and the third makes the element a value of
// its own. See SubscriptedArrayLiteralPolicy.
func (r *Runner) assignElemLiteral(a *syntax.Assign) {
	switch r.subscriptedArrayLiteral() {
	case SubscriptedArrayLiteralRefused:
		// The subscript as written rather than as evaluated: the refusing
		// shell quotes `a[$i]` back, and it refuses before evaluating it at
		// all — `a[1/0]=(p q)` is the same sentence there and not a division
		// by zero, so nothing is asked of the subscript on this route.
		//
		// So the subscript is not expanded to quote it either, where the
		// source holds its text: `a[$(f)]=(p q)` does not run `f` (#6674).
		subject := r.writtenIndex(a)
		if subject == "" {
			subject = r.subscriptAsWritten(a.Index)
		}
		r.failedSubscript("%s\n", Wording(r.diag().ArrayLiteralThroughASubscript,
			"%[1]s[%[2]s]: cannot assign list to array member", a.Name, subject))
	case SubscriptedArrayLiteralSplices:
		if r.assignWholeArrayElemLiteral(a) {
			return
		}
		r.spliceElemLiteral(a)
	case SubscriptedArrayLiteralNests:
		if r.assignWholeArrayElemLiteral(a) {
			return
		}
		r.nestElemLiteral(a)
	}
	// Unspecified has already been reported by name, and r.unspecified is set
	// so nothing after it writes.
}

// assignWholeArrayElemLiteral is `a[@]=(p q)` and `a[*]=(p q)`: the
// whole-array spelling with a literal on the right. It reports whether it
// answered the line; where it did not, the element road goes on as it would.
//
// The subscript names no element, so neither element road can take it: the
// splice and the nest each read it as arithmetic, met a character with no
// operand, and complained about the expression — and `*` was globbed against
// the working directory on the way. It is the same spelling the scalar road
// answers in Runner.assignWholeArraySubscript, so it asks the same two fields.
// Measured 2026-10-16 under `-c`, after `a=(1 2)`:
//
//	written                       zsh 5.9.2                ksh93u+
//	a[@]=(p $(echo VAL >&2))      VAL, `a=( p )`, 0        `@: invalid subscript
//	a[*]=(p q)                    `a=( p q )`, 0           in assignment`, no
//	a[@]+=(p q)                   `a=( 1 2 p q )`          VAL, and the input
//	unset a; a[@]=(p q)           `a=( p q )`              ends — in every row,
//	a[@]=([5]=x y)                four empty, `x y`        over a table too
//
// bash refuses the list before it reads the subscript, on the Refused road,
// so it never reaches here.
//
// The taking column takes it as the whole name, as its scalar road does, but
// not over a scalar or a table: `s=x; s[@]=(p q)` is `attempt to assign array
// value to non-array` and a table is `attempt to set slice of associative
// array`, both after the literal is expanded — the splice's own two refusals,
// so the splice's gate answers them.
func (r *Runner) assignWholeArrayElemLiteral(a *syntax.Assign) bool {
	policy, whole := r.wholeArrayLiteralPolicy(a)
	if !whole {
		return false
	}
	switch policy {
	case WholeArraySubscriptNamesEveryElement:
		words, ok := r.literalWords(a.Name, a.Elems)
		if !ok {
			return true
		}
		if !r.spliceTargetIsAnArray(a) || r.refuseTheHeldFreeze(a) {
			return true
		}
		if a.Append {
			elems, _ := r.arrayElemsOfTheName(a.Name)
			words = append(append([]string{}, elems...), words...)
		}
		r.setArray(a.Name, words)
		return true
	case WholeArraySubscriptIsInvalidInAnAssignment, WholeArraySubscriptIsABadSubscript:
		// Refused before the literal is expanded, as the scalar road
		// refuses it before its value is.
		r.refuseWholeArraySubscript(a, policy)
		return true
	case WholeArraySubscriptAssignUnspecified:
		r.diagf("%s\n", r.unanswered("the whole-array subscript on the left of an assignment"))
		r.status, r.unspecified = 2, true
		return true
	}
	// A key named `@`, or a table's slice: the element road already answers
	// both, the second through the splice's gate.
	return false
}

// wholeArrayLiteralPolicy reports whether a subscripted literal or compound
// body is written with the whole-array spelling, and which of the two fields
// answers it for this name.
func (r *Runner) wholeArrayLiteralPolicy(a *syntax.Assign) (WholeArraySubscriptAssignPolicy, bool) {
	if a.IndexFlags != nil || a.IndexRange != nil || len(a.Leading) > 0 ||
		!wholeArraySubscript(a.IndexText) {
		return WholeArraySubscriptAssignUnspecified, false
	}
	if r.assocDeclared(a.Name) {
		return r.sem().WholeArraySubscriptAssigningATable, true
	}
	return r.sem().WholeArraySubscriptAssigningAnArray, true
}

// refusedWholeArrayCompound is the whole-array spelling with a compound body
// on the right — `a[@]=(x=1)`, and `a[@]=()` in the grammar that reads an
// empty list as one. Measured 2026-10-16 on ksh93u+, the one column with the
// construct, under `-c`: `@: invalid subscript in assignment` for both, and
// over a table too, with the input ended and nothing in the body expanded —
// the literal's own answer. Only a refusal is taken here; nothing measured
// says what a taking column would do with a body.
func (r *Runner) refusedWholeArrayCompound(a *syntax.Assign) bool {
	policy, whole := r.wholeArrayLiteralPolicy(a)
	if !whole || policy != WholeArraySubscriptIsInvalidInAnAssignment &&
		policy != WholeArraySubscriptIsABadSubscript {
		return false
	}
	r.refuseWholeArraySubscript(a, policy)
	return true
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
	build, ok := elementLiteralBuilt(r, a, func() ([]string, bool) {
		words, ok := r.literalWords(a.Name, a.Elems)
		if ok {
			r.openTraceTakesTheLiteral(a, words)
		}
		return words, ok
	})
	if !ok {
		return
	}
	if a.IndexRange != nil {
		r.spliceElemLiteralOverAFlaggedSpan(a, build)
		return
	}
	if a.IndexFlags != nil {
		r.spliceElemLiteralAtAFlaggedPlace(a, build)
		return
	}
	r.spliceWordsIntoElement(a, func() string { return r.joinWord(a.Index) }, build)
}

// elementLiteralBuilt arranges when an element literal's value is built
// against its subscript, and hands back what the store calls for the value
// at the point where it writes.
//
// The two are not interleaved: each column does the whole of one side
// before the other. Measured 2026-10-15 on zsh 5.9.2 and ksh93u+
// 2012-08-01 under `-c`, after `a=(1 2)`:
//
//	written                                  zsh               ksh93
//	a[$(echo SUB >&2; echo 1)]=(p $(…VAL…))  VAL SUB, `p 2`    SUB VAL, `1 (p)`
//	a[$(…SUB…; echo 1/0)]=(p $(…VAL…))       VAL SUB, 1/0      SUB, 1/0
//	a[$(…SUB…; echo 1)]=(p $((1/0)))         1/0               SUB, 1/0
//	i=1; a[i++]=(p $((i*10)))                `p 10 2`          `1 (p 20)`
//
// So the subscript's arithmetic goes with its words here, where a scalar
// element write evaluates it after the value in every column — which is
// why this is Semantics.ElementSubscriptIsExpandedBeforeTheValue's answer
// and not a second axis: the axis says which side comes first, and an
// element literal does all of that side first. A table's key in ksh93 is
// the same row: `h[$(…SUB…)]=(p $(…VAL…))` is SUB VAL.
//
// The store's call goes through the held freeze, so a frozen name is
// refused there, with both sides done. See interp/frozenorder.go (#6675,
// #6674).
func elementLiteralBuilt[T any](r *Runner, a *syntax.Assign, build func() (T, bool)) (func() (T, bool), bool) {
	first := r.subscriptExpandsBeforeTheValue(a)
	if r.unspecified {
		return nil, false
	}
	if !first {
		v, ok := build()
		if !ok {
			return nil, false
		}
		build = func() (T, bool) { return v, true }
	}
	return func() (T, bool) {
		v, ok := build()
		if !ok || r.refuseTheHeldFreeze(a) {
			var none T
			return none, false
		}
		return v, true
	}, true
}

// spliceWordsIntoElement is that splice with the words handed in, because two
// constructs reach it and only one of them is a literal.
//
// The second is a **pattern on the right of a subscripted assignment** in the
// shell that reads one — `setopt globassign; a=(q w e); a[2]=*.txt` is five
// elements there, the match taking the element's place and pushing the rest
// along. That is the same operation with the same ends, the same padding, the
// same append rule and the same two refusals, so it is the same function
// rather than a second one that would come to disagree with it. See
// Runner.globbedElementIsAList.
//
// The words are a function rather than a slice because the store calls for
// them at the point where it writes, after the target has been refused and
// after the subscript has been read, in both branches below: a dialect that
// builds a literal's value after its subscript builds it there, and the
// call is where a frozen name is refused. One that builds it first hands in
// what it has already built — see elementLiteralBuilt.
//
// The subscript is a function too, called once the target has been
// accepted, because the column that splices refuses a table's slice before
// it reads the subscript: `typeset -A h; h[$(f)]=(p q)` never runs `f`. It
// is read **once**, and that is not tidiness: the caller on the pattern
// road has already read it and hands in what it got, and joining the word a
// second time runs its substitutions a second time — `a[$(f)]=*.txt` ran
// `f` twice (#1915 again, one construct over).
func (r *Runner) spliceWordsIntoElement(a *syntax.Assign, subscript func() string, build func() ([]string, bool)) {
	if !r.spliceTargetIsAnArray(a) {
		return
	}
	text := subscript()
	// The subscript as written is what a boundary refusal quotes back, and it
	// is not the text the arithmetic reads — see subscriptSubject (#1373).
	subject := subscriptSubject(r.writtenIndex(a), text)
	from, to, outcome := r.assignSpan(a, text)
	if outcome == spanReported {
		return
	}
	if outcome == spanResolved {
		// A *range* of elements rather than one: the words replace the whole
		// span, so `a=(1 2 3); a[2,3]=(x y)` is three elements and not four.
		// See spliceElementSpan for what each end is measured to do.
		words, ok := build()
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
	words, ok := build()
	if !ok {
		return
	}

	elems, _ := r.arrayElemsOfTheName(a.Name)
	pos, within := r.elemPos(r.arrayForWrite(a.Name), idx)
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
	if _, isArray := r.arrayElemsOfTheName(name); isArray {
		return true
	}
	if a.Operand && r.attrHas(name, attrDeclaredEmpty) {
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
func (r *Runner) literalWords(name string, elems []*syntax.ArrayElem) ([]string, bool) {
	parsed, ok := r.literalElems(elems, r.literalShapeReadsSubscripts(elems), false)
	if !ok {
		// A failed element list costs the splice, exactly as it costs the
		// whole-array spelling — see literalElems.
		return nil, false
	}
	built, ok := r.literalInto(name, NewArray(0), 0, parsed, false)
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
//
// **Produced as well as stored**, which is the half it was missing. A name
// whose elements come from a producer has nothing in the array table, so
// asking the table alone said such a name was not an array at all — and since
// this is the reading every caller here uses to decide *whether* a name is
// one, that answer arrived as `attempt to assign array value to non-array`,
// the sentence a scalar gets. One dialect produces its positional parameters
// under a name, so `argv[2]=(p q)` was refused where `a[2]=(p q)` spliced, and
// `shift argv` and `argv[@]+=x` reached nothing for the same reason (#4614).
//
// The second value is the answer to "is this an array", so it must stay
// distinguishable from an array holding no elements: a producer reporting
// nothing is still a producer, and `true` is the honest answer for it.
func (r *Runner) arrayElemsOfTheName(name string) ([]string, bool) {
	if a, stored := r.arraysLookup(name); stored {
		return r.readArray(a), true
	}
	if produce, produced := r.arrayProducersLookup(name); produced {
		return produce(r), true
	}
	// And the pipeline record, the fourth kind, produced by a path of its
	// own: `true|false; pipestatus[1,2]=(8)` splices on zsh 5.9.2 where this
	// refused it as a scalar (#6088).
	if elems, record := r.pipelineStatuses(name); record {
		return elems, true
	}
	return nil, false
}
