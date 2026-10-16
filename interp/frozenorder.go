// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Where an assignment to a frozen name is refused.
//
// The refusal used to stand in front of everything Runner.assign does, so a
// frozen name was refused before a single word of the assignment had been
// expanded. No shell in the panel does that for a scalar, and none does it
// for an element. Measured 2026-10-15 against bash 5.3.20, zsh 5.9.2 and
// ksh93u+ 2012-08-01, under `-c` and from a file, the side effects on
// standard error:
//
//	readonly x=1; x=$(echo VAL >&2)y              all three: VAL, refusal
//	readonly x=1; x=$((1/0))                      all three: the division,
//	                                              and no refusal
//	readonly x=1; set -u; x=$nope                 all three: `nope`, and no
//	                                              refusal
//	readonly a=(1 2)
//	  a[$(echo SUB >&2; echo 1)]=$(…VAL…)x        bash VAL SUB, zsh VAL,
//	                                              ksh93 SUB VAL; then the
//	                                              refusal
//	  a[$(echo SUB >&2; echo 1/0)]=$(…VAL…)       bash and ksh93: the
//	                                              division; zsh: the refusal
//	  a=($(echo VAL >&2))                         zsh VAL; bash, ksh93
//	                                              nothing; then the refusal
//	  a[$(…SUB…)]=(p $(…VAL…))                    bash refuses the list as
//	                                              it refuses one anywhere;
//	                                              zsh VAL SUB, then the
//	                                              refusal; ksh93 SUB VAL
//
// So a scalar is refused once its value is expanded, and a value that will
// not expand is what is reported. An element write splits on whether the
// subscript is reached: zsh refuses once the value is expanded and bash and
// ksh93 at the store, after the subscript's words in each one's own order
// (see interp/elementorder.go) and its arithmetic. A whole literal splits on
// whether its words are expanded first. An element literal reaches its
// store in every column — `set -x` changes none of it.
//
// The two splits are Semantics.FrozenElementIsRefusedBeforeItsSubscript and
// Semantics.FrozenLiteralIsExpandedBeforeItIsRefused. The rest is core.
//
// "At the store" is a held refusal rather than a second reading of the
// subscript: the store's own road expands and evaluates it, and asks
// Runner.refuseTheHeldFreeze at the point where it would write. A road
// that has no such point is refused up front, where it always was — see
// Runner.frozenElementWaitsForItsStore for which ones those are (#6674).

// heldFreeze is a refusal waiting for an assignment's store: the
// assignment, and the frozen name it lands on.
type heldFreeze struct {
	assign *syntax.Assign
	name   string
}

// frozenOutcome is what Runner.refuseAFrozenAssignment decided.
type frozenOutcome int

const (
	// The assignment goes on: nothing about the name refuses it.
	freezeDoesNotRefuse frozenOutcome = iota
	// The assignment is over: refused, or something in it failed and was
	// reported in the refusal's place.
	freezeRefusedIt
	// The assignment goes on to its store, which refuses it there. See
	// Runner.refuseTheHeldFreeze.
	freezeWaitsForTheStore
)

// refuseAFrozenAssignment is the freeze's part of Runner.assign: it expands
// what the dialect expands before refusing a frozen name, then refuses it —
// or hands the refusal to the store, which is the last thing there is to
// expand before.
func (r *Runner) refuseAFrozenAssignment(a *syntax.Assign) (frozenOutcome, string) {
	name := r.assignmentLandsOn(a.Name)
	if r.refusePrivateWrite(name) {
		return freezeRefusedIt, name
	}
	if !r.freezeRefuses(name, assignedAlone) {
		return freezeDoesNotRefuse, name
	}
	if r.assignmentGaveUp() {
		// Something the trace expanded ahead of the store has failed and
		// been reported, which is what the shell says in the refusal's
		// place.
		return freezeRefusedIt, name
	}
	switch {
	case a.Operand || a.Members != nil || len(a.Leading) > 0:
		// A declaration's operand was expanded by the declaration, and a
		// compound body and a chain of subscripts have stores of their own
		// that nothing here has measured. Refused up front, as they were.
	case a.IsArray && a.Index != nil:
		// An element literal reaches its store in every column, or the
		// refusal of the construct itself where there is one.
		return freezeWaitsForTheStore, name
	case a.IsArray:
		if r.frozenLiteralIsExpandedFirst(a) {
			r.literalElems(a.Elems, r.literalShapeReadsSubscripts(a.Elems), false)
			if r.assignmentGaveUp() {
				return freezeRefusedIt, name
			}
		}
	case a.Index == nil:
		r.expandTheValueAheadOfTheFreeze(a)
		if r.assignmentGaveUp() {
			return freezeRefusedIt, name
		}
	default:
		switch r.frozenElementOrder(a) {
		case frozenElementAfterTheValue:
			r.expandTheValueAheadOfTheFreeze(a)
			if r.assignmentGaveUp() {
				return freezeRefusedIt, name
			}
		case frozenElementAtTheStore:
			return freezeWaitsForTheStore, name
		}
	}
	if r.unspecified {
		return freezeRefusedIt, name
	}
	r.refuseAsAFreezeWould(name, assignedAlone, "")
	return freezeRefusedIt, name
}

// refuseTheHeldFreeze is the store's half of a held refusal: true when this
// assignment's store is refused, having said so. Every store a held
// assignment can reach asks it at the point where it would write, after
// the last thing it expands or evaluates.
func (r *Runner) refuseTheHeldFreeze(a *syntax.Assign) bool {
	h := r.heldFreeze
	if h.assign == nil || h.assign != a {
		return false
	}
	r.heldFreeze = heldFreeze{}
	r.refuseAsAFreezeWould(h.name, assignedAlone, "")
	return true
}

// expandTheValueAheadOfTheFreeze expands a scalar value the way its store
// would, for its side effects and its failure: the refusal that follows
// stores nothing. A value the trace has already expanded is not expanded
// again, and one that can neither run nor fail anything is not expanded at
// all, since nobody could tell.
func (r *Runner) expandTheValueAheadOfTheFreeze(a *syntax.Assign) {
	if _, held := r.heldAssignValue(a); held || !r.wordTouchesState(a.Value) {
		return
	}
	r.assignValue(a)
}

// frozenElementOrder is where an element write to a frozen name is refused.
type frozenElementOrder int

const (
	// Before anything is expanded, because nothing that could be seen is.
	frozenElementNow frozenElementOrder = iota
	// Once the value is expanded, and before the subscript is.
	frozenElementAfterTheValue
	// At the store, after the subscript too.
	frozenElementAtTheStore
)

// frozenElementOrder answers Semantics.FrozenElementIsRefusedBeforeItsSubscript
// for this write, asking it only where the answer could be seen.
func (r *Runner) frozenElementOrder(a *syntax.Assign) frozenElementOrder {
	before := r.sem().FrozenElementIsRefusedBeforeItsSubscript
	if before == Unspecified {
		if !r.frozenElementOrderIsObservable(a) {
			return frozenElementNow
		}
		if r.ask(before, "a frozen element write refused before its subscript is expanded") {
			before = Yes
		} else {
			before = No
		}
	}
	if r.unspecified {
		return frozenElementNow
	}
	if before == Yes {
		return frozenElementAfterTheValue
	}
	if r.frozenElementWaitsForItsStore(a) {
		return frozenElementAtTheStore
	}
	return frozenElementNow
}

// frozenElementOrderIsObservable reports whether refusing a frozen element
// write up front could differ from refusing it later: the value or the
// subscript can run or fail something. Deliberately wide, for the reason
// elementOrderIsObservable is.
//
// The whole-array spelling is always observable: where its own refusal
// comes first, the sentence is a different one even when nothing in the
// assignment runs — over a table too, where `@` is no arithmetic for the
// last clause to fail on (#6761).
func (r *Runner) frozenElementOrderIsObservable(a *syntax.Assign) bool {
	return r.wordTouchesState(a.Value) || r.wordTouchesState(a.Index) ||
		a.IndexFlags == nil && wholeArraySubscript(a.IndexText) ||
		!r.assocDeclared(a.Name) && r.subscriptArithTouchesState(a.Index)
}

// subscriptArithTouchesState reports whether an indexed subscript's
// expression, read as arithmetic once its expansions are in, can store,
// step or fail: the written part of `a[i++]` or `a[$i/0]`. Each expansion
// stands in as a number, since what it does is wordTouchesState's question.
func (r *Runner) subscriptArithTouchesState(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	var b []byte
	for _, s := range w.Spans {
		if s.Kind == syntax.Literal {
			b = append(b, s.Value...)
			continue
		}
		b = append(b, " 0 "...)
	}
	return r.arithTextTouchesState(string(b))
}

// frozenElementWaitsForItsStore reports whether this element write's store
// asks Runner.refuseTheHeldFreeze: the plain subscript over an indexed array
// or a table, and the whole-array spelling. A flag group, a pair of ends and
// a chain each have a road that does not, and are refused up front as they
// always were.
//
// The whole-array spelling waits because its own refusal comes first where
// it has one (#6761). Measured 2026-10-16 on bash 5.3.20 and ksh93u+, `-c`
// and file, after `readonly a=(1 2)`: `a[@]=$(echo VAL >&2)x` is VAL and
// `a[@]: bad array subscript` in bash and `@: invalid subscript in
// assignment`, with no VAL, in ksh93 — the sentences each writes over a name
// that is not frozen. Over a frozen table bash stores `@` as a key, so there
// the store is reached and refused for the freeze, after VAL.
func (r *Runner) frozenElementWaitsForItsStore(a *syntax.Assign) bool {
	return a.IndexFlags == nil && a.IndexRange == nil && len(a.Leading) == 0 &&
		!r.scalarAssignmentGlobs()
}

// frozenLiteralIsExpandedFirst answers
// Semantics.FrozenLiteralIsExpandedBeforeItIsRefused for this literal,
// asking it only where something in the literal could be seen to run or
// fail.
func (r *Runner) frozenLiteralIsExpandedFirst(a *syntax.Assign) bool {
	first := r.sem().FrozenLiteralIsExpandedBeforeItIsRefused
	if first != Unspecified {
		return first == Yes
	}
	if !r.literalTouchesState(a.Elems) {
		return false
	}
	return r.ask(first, "a frozen name's array literal expanded before it is refused")
}

// frozenLiteralIsRefusedBeforeItsWords reports whether this whole literal
// lands on a frozen name that refuses it before any of its words is
// expanded — which the trace asks before expanding them to print, so that
// `set -x` does not run what the refusal would not. Measured 2026-10-15 on
// ksh93u+, `readonly a=(1 2); set -x; a=($(echo VAL >&2))` writes the
// refusal and nothing else, traced as untraced.
func (r *Runner) frozenLiteralIsRefusedBeforeItsWords(a *syntax.Assign) bool {
	if a.Operand || !r.freezeRefuses(r.assignmentLandsOn(a.Name), assignedAlone) {
		return false
	}
	return !r.frozenLiteralIsExpandedFirst(a)
}

// literalTouchesState is wordTouchesState for a literal's elements, a
// subscripted element's written subscript read as arithmetic as well — see
// elementHeadTouchesState.
func (r *Runner) literalTouchesState(elems []*syntax.ArrayElem) bool {
	for _, e := range elems {
		if e.Nested != nil && r.literalTouchesState(e.Nested.Elems) {
			return true
		}
		if e.Word == nil {
			continue
		}
		if r.wordTouchesState(e.Word) ||
			syntax.SubscriptedElement(e.Word) && r.elementHeadTouchesState(e.Word) {
			return true
		}
	}
	return false
}

// elementHeadTouchesState reports whether a `[sub]=` head's subscript can
// store, step or fail when it is evaluated: `[i++]=v` can and `[z]=9` cannot.
// A head whose subscript is not one literal run is counted, which is the
// wide side to err on.
func (r *Runner) elementHeadTouchesState(w *syntax.Word) bool {
	if len(w.Spans) == 0 || w.Spans[0].Kind != syntax.Literal {
		return true
	}
	v := w.Spans[0].Value
	end := strings.IndexByte(v, ']')
	if !strings.HasPrefix(v, "[") || end < 0 {
		return true
	}
	return r.arithTextTouchesState(v[1:end])
}
