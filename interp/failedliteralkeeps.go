// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"

	"github.com/blairham/sh/syntax"
)

// failedLiteral is where a literal that failed partway through was written,
// which is what decides what of it is kept.
type failedLiteral struct {
	// appendTo is `+=`.
	appendTo bool
	// operand says the literal is a declaration utility's operand rather
	// than a bare assignment.
	operand bool
	// letter is the container letter the declaration carries, 'a' or 'A',
	// and 0 where it carries neither or is not a declaration.
	letter byte
	// refuseBare is the table store's answer for a bare element among
	// heads — see Runner.tableLiteralShape.
	refuseBare bool
}

// keepWhatAFailedLiteralStored is what is left of a name after an array
// literal fails to expand partway through. In most columns, for most
// literals, it is what the name held before, and this returns having changed
// nothing. Three readings keep something, each asked only once a literal has
// failed, so no literal that expands asks anything:
//
//   - a literal whose first element names its subscript, built as a table a
//     pair at a time — Semantics.FailedSubscriptedLiteralKeepsWhatItStored
//     (#6608);
//   - a bare append onto a table, plain pairs included —
//     Semantics.FailedTableAppendKeepsThePairsItAdded (#6704);
//   - a declaration whose container letter re-creates the name before its
//     literal fails — Semantics.FailedOperandLeavesTheLettersKind (#6705);
//   - a bare literal over a name that is not an array yet, which is made one
//     before its elements fail and keeps none of them —
//     Semantics.FailedBareLiteralStillMakesTheNameAnArray (#7078).
//
// parsed is the elements before the failed one — literalElems trims the one
// that failed.
func (r *Runner) keepWhatAFailedLiteralStored(
	name string,
	elems []*syntax.ArrayElem,
	parsed []literalElem,
	how failedLiteral,
) {
	if name == "" || r.unspecified || len(elems) == 0 {
		// The list was refused by an axis no dialect answered, and nothing
		// about it is evidence.
		return
	}
	if elems[0].Word != nil && syntax.SubscriptedElement(elems[0].Word) &&
		r.keepWhatASubscriptedLiteralPlaced(name, parsed, how) {
		return
	}
	if r.unspecified {
		return
	}
	if how.operand {
		if how.letter != 0 && !how.appendTo &&
			r.ask(r.sem().FailedOperandLeavesTheLettersKind,
				"a declaration's container letter re-creating a name whose literal failed") {
			r.startOverAsTheLettersKind(name, how.letter)
		}
		return
	}
	if r.assocDeclared(name) {
		if how.appendTo && r.ask(r.sem().FailedTableAppendKeepsThePairsItAdded,
			"a failed append onto a table keeping the pairs it had added") {
			r.assignAssocElems(name, withoutADanglingKey(parsed), true, how.refuseBare)
		}
		return
	}
	if _, isArray := r.arrayToAppendTo(name); isArray ||
		r.isCompoundVariable(name) || r.attrHas(name, attrReadonly) {
		// An array already, which every column leaves holding what it held;
		// or a name nothing may convert.
		return
	}
	if r.ask(r.sem().FailedBareLiteralStillMakesTheNameAnArray,
		"a bare array literal that failed still making its name an array") {
		// What an append would have built on, which is the name as an array
		// with none of the literal in it: empty where it was unset, its
		// scalar at element zero where it held one.
		a, _ := r.appendedOverAScalar(name)
		r.storeArray(name, a)
	}
}

// keepWhatASubscriptedLiteralPlaced is the first of those readings, and
// reports whether it applied.
//
// A declaration carrying the indexed-array letter keeps the pairs as an
// indexed array rather than as a table: `typeset -a a=([2]=x [0]=$((1/0)))`
// is `typeset -a a=([2]=x)`.
func (r *Runner) keepWhatASubscriptedLiteralPlaced(name string, parsed []literalElem, how failedLiteral) bool {
	appendTo, indexedLetter := how.appendTo, how.letter == 'a'
	if !r.ask(r.sem().FailedSubscriptedLiteralKeepsWhatItStored,
		"a failed subscripted array literal keeping what it had stored") {
		return false
	}
	table := r.assocDeclared(name)
	if appendTo {
		// An append onto a table has added the pairs before the failure; one
		// onto an indexed array has added nothing.
		if table {
			r.assignAssocElems(name, parsed, true, false)
		}
		return true
	}
	if indexedLetter && !table {
		// The declaration's letter decides the kind, so the pairs land by
		// their evaluated subscripts in a fresh indexed array.
		if built, ok := r.literalInto(name, NewArray(0), 0, parsed, true); ok {
			r.storeArray(name, built)
		}
		return true
	}
	// The name is started over as a table holding what was placed, whatever
	// it was before — an indexed array, a scalar, a name with a type letter.
	r.clearTypeAttributes(name)
	r.markAssoc(name)
	r.assignAssocElems(name, parsed, false, false)
	return true
}

// withoutADanglingKey is a failed table literal's completed pairs: the
// elements before the failure, less a bare key whose value never came.
// Measured 2026-10-15 on bash 5.3.20, after `m=([k]=old)`: `m+=(j new i
// $((1/0)))` keeps `j` and not `i`, where a successful `m+=(j new i)` stores
// `i` empty — and `m+=(j new i $(echo SIDE >&2) $((1/0)))` keeps `i` empty,
// because the empty substitution is its value.
func withoutADanglingKey(parsed []literalElem) []literalElem {
	bare := 0
	for _, e := range parsed {
		if !e.subscripted {
			bare += len(e.fields)
		}
	}
	if bare%2 == 0 {
		return parsed
	}
	out := slices.Clone(parsed)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i].subscripted {
			continue
		}
		e := out[i]
		e.fields = e.fields[:len(e.fields)-1]
		if len(e.fields) == 0 {
			return slices.Delete(out, i, i+1)
		}
		out[i] = e
		return out
	}
	return out
}

// startOverAsTheLettersKind is what a declaration carrying a container
// letter leaves of a name whose literal failed, in the column where the
// letter re-creates the name before the literal is stored: the name, of
// that kind, with no value and no other attribute. Measured 2026-10-15 on
// ksh93u+ 2012-08-01 from a file, read by an exit trap:
//
//	a=(1 2); typeset -a a=(5 $((1/0)))                typeset -a a
//	typeset -A m=([k]=old); typeset -A m=(j new i …)  typeset -A m=()
//	a=(1 2); typeset -A a=(5 …)                       typeset -A a=()
//	typeset -A m=([k]=old); typeset -a m=(5 …)        typeset -a m
//	export a=x; typeset -a a=(5 …)                    typeset -a a
//	typeset -i s=3; typeset -a s=(5 …)                typeset -a s
//	unset a; typeset -a a=(5 …)                       typeset -a a
//	a=(1 2); typeset a=(5 …)                          typeset -a a=(1 2)
//	a=(1 2); typeset -a a+=(5 …)                      typeset -a a=(1 2)
//
// So it is the letter and not the literal — with no letter the name keeps
// what it held — and an append does not start anything over.
func (r *Runner) startOverAsTheLettersKind(name string, letter byte) {
	r.unsetName(name)
	if letter == 'A' {
		r.markAssoc(name)
		r.assocsPut(name, AssocArray{})
		return
	}
	r.storeArray(name, NewArray(0))
}
