// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The order an element write expands its pieces in.
//
// `a[$(f)]=$(g)` has two words to expand and, for an indexed name, an
// expression to evaluate, and the panel agrees on one part of the order and
// splits on the other. Measured 2026-10-15 against bash 5.3.20, zsh 5.9.2 and
// ksh93u+ 2012-08-01 under `-c` and a script file, with the side effects on
// standard error:
//
//	a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)x        bash, zsh: VAL SUB
//	                                                    ksh93:     SUB VAL
//	a[$(echo SUB >&2; echo 1)]=$(echo VAL >&2)$((1/0)) bash, zsh: VAL, complaint
//	                                                    ksh93:     SUB VAL, complaint
//	a[$(echo SUB >&2; echo 1/0)]=$(echo VAL >&2)y      all three: the value,
//	                                                    then the complaint
//	i=1; a[i++]=$((i*10))                              all three: element 1
//	                                                    holds 10
//	i=1; a[$((i++))]=$((i*10))                          bash, zsh: 10; ksh93: 20
//
// So the subscript's *arithmetic* is evaluated after the value in every
// column — the third and fourth rows, where ksh93 too expands the value
// before the expression that will not evaluate is reported, and `i++` has not
// yet stepped when the value reads `i`. What splits is only the subscript's
// *word expansion*: ksh93 performs it before the value and bash and zsh
// after. A table's key is nothing but that expansion, so `typeset -A m;
// m[$(echo SUB >&2; echo k)]=$(echo VAL >&2)z` is `VAL SUB` in bash and zsh
// and `SUB VAL` in ksh93, and a chain `a[$(…)][$(…)]=$(…)` in ksh93 expands
// both subscripts, then the value, then evaluates them. `set -x` changes
// none of it: `+ echo SUB` is traced before `+ echo VAL` in ksh93 and after
// it in the other two. See Semantics.ElementSubscriptIsExpandedBeforeTheValue.
//
// Both stores here used to answer one fixed way, and the two did not agree
// with each other: an untraced write expanded the subscript first and a
// traced one expanded the value first, in every dialect, so `set -x` changed
// the order (#6604).

// elementWriteHasAValueToOrder reports whether this assignment is an element
// write whose scalar value is an expansion, which is the only shape the order
// is a question about. A literal value runs nothing, and an element literal
// (`a[1]=(p q)`) or a compound body is a different construct with its own
// stores.
func elementWriteHasAValueToOrder(a *syntax.Assign) bool {
	return a.Index != nil && !a.IsArray && a.Members == nil && wordCanWrite(a.Value)
}

// subscriptExpandsBeforeTheValue reports whether this element write expands
// its subscript's words before its value — see the table above.
//
// A dialect that answers the axis is taken at its word on every element
// write. One that does not is asked only where the order can be seen: two
// expansions where one of them can run or fail something. `a[$i]=$x` reads
// two names and nothing else, so either order is the same store and the
// core leaves it unasked; `a[$(f)]=$(g)` is the disagreement. See
// elementOrderIsObservable.
func (r *Runner) subscriptExpandsBeforeTheValue(a *syntax.Assign) bool {
	switch r.sem().ElementSubscriptIsExpandedBeforeTheValue {
	case Yes:
		return true
	case No:
		return false
	}
	if !r.elementOrderIsObservable(a) {
		return false
	}
	return r.ask(r.sem().ElementSubscriptIsExpandedBeforeTheValue,
		"an element write's subscript expanded before its value")
}

// valueAheadOfTheSubscript expands an element write's value before its
// subscript, for the dialects that do, and hands back what the store then
// reads it from. Nil means the store expands the value itself, at its own
// point, which is after the subscript's words — and that is the other
// answer.
//
// Nothing is expanded twice: the value goes where Runner.assignValue looks
// first, the arrangement the trace has used since #1915.
func (r *Runner) valueAheadOfTheSubscript(a *syntax.Assign) *expandedAssign {
	if !elementWriteHasAValueToOrder(a) {
		return nil
	}
	if r.expanded != nil && r.expanded.assign == a {
		// The trace's loop has decided this one already, one way or the
		// other — see Runner.assignAll.
		return nil
	}
	if r.subscriptExpandsBeforeTheValue(a) || r.unspecified {
		return nil
	}
	e := &expandedAssign{assign: a}
	e.value, e.fields, e.fieldsSet = r.expandScalarAssignValue(a.Value)
	return e
}

// heldAssignValue is the value the caller expanded for this assignment, if
// there is one — expanding it now if the caller left it to the store, which
// is how a traced write that expands its subscript first gets its value
// after the subscript and still exactly once.
func (r *Runner) heldAssignValue(a *syntax.Assign) (*expandedAssign, bool) {
	e := r.expanded
	if e == nil || e.assign != a {
		return nil, false
	}
	if e.valuePending {
		e.value, e.fields, e.fieldsSet = r.expandScalarAssignValue(a.Value)
		e.valuePending = false
	}
	return e, true
}

// elementOrderIsObservable reports whether the two orders could differ for
// this element write: one side can run or fail something and the other has
// anything to expand at all.
//
// Deliberately a little wide — a command substitution on one side and a
// plain `$x` on the other is counted, though only the substitution's own
// output order differs — and never narrow: the cost of asking where nothing
// differs is one question a script with no dialect did not need, and the
// cost of not asking where something does is a silent answer in one column's
// order.
func (r *Runner) elementOrderIsObservable(a *syntax.Assign) bool {
	subTouches, subExpands := false, false
	for _, link := range a.Leading {
		subTouches = subTouches || r.wordTouchesState(link.Index)
		subExpands = subExpands || wordCanWrite(link.Index)
	}
	subTouches = subTouches || r.wordTouchesState(a.Index)
	subExpands = subExpands || wordCanWrite(a.Index)
	if a.IsArray {
		// An element literal's subscript is evaluated on its own side of
		// the list, arithmetic and all — see Runner.elementLiteralOrder —
		// so `a[i++]=(p $i)` is a question where `a[i++]=$i` is not.
		subTouches = subTouches || !r.assocDeclared(a.Name) && r.subscriptArithTouchesState(a.Index)
		return subTouches || r.literalTouchesState(a.Elems) && (subExpands || a.IndexText != "" && !isDecimalSubscript(a.IndexText))
	}
	if !subExpands {
		// A subscript with nothing to expand is only evaluated, and that is
		// after the value in every column.
		return false
	}
	return subTouches || r.wordTouchesState(a.Value)
}

// wordTouchesState reports whether expanding this word could run a command,
// change a variable, read a parameter that changes as it is read, or fail.
func (r *Runner) wordTouchesState(w *syntax.Word) bool {
	if w == nil {
		return false
	}
	for _, s := range w.Spans {
		switch s.Kind {
		case syntax.Literal:
		case syntax.ParamExp:
			if r.paramTouchesState(s.Param) {
				return true
			}
		case syntax.ArithSubst:
			if r.arithTextTouchesState(s.Value) {
				return true
			}
		default:
			return true
		}
	}
	return false
}

// paramTouchesState is wordTouchesState for one `${…}`.
func (r *Runner) paramTouchesState(e *syntax.ParamExpr) bool {
	if e == nil || r.nounset {
		// Unparsed, or any name at all can be the one that is not set.
		return true
	}
	switch e.Op {
	case syntax.ParamAssign, syntax.ParamAssignAlways, syntax.ParamError, syntax.ParamSubstring:
		// A store, a refusal, or a range whose ends are arithmetic.
		return true
	}
	if e.Indirect || e.HasFlags || r.DynamicParameter(e.Name) {
		return true
	}
	if e.Index != nil && !r.positionSubscriptRunsNothing(e) {
		return true
	}
	if e.Inner != nil && r.wordTouchesState(e.Inner) {
		return true
	}
	return r.wordTouchesState(e.Arg) || r.wordTouchesState(e.Arg2)
}

// arithTextTouchesState is wordTouchesState for a `$(( … ))`: whether the
// expression can store, step, divide (and so fail), expand, or read a
// parameter that changes as it is read. `$((i+1))` is none of those.
func (r *Runner) arithTextTouchesState(t string) bool {
	if r.nounset || strings.Contains(t, "++") || strings.Contains(t, "--") {
		return true
	}
	for i := 0; i < len(t); {
		c := t[i]
		switch {
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i + 1
			for j < len(t) && (t[j] == '_' || t[j] >= 'a' && t[j] <= 'z' ||
				t[j] >= 'A' && t[j] <= 'Z' || t[j] >= '0' && t[j] <= '9') {
				j++
			}
			if r.DynamicParameter(t[i:j]) {
				return true
			}
			i = j
		case c == '=':
			// `==` is a comparison and every other `=` is a store — `=`,
			// `+=`, `<<=` — except the second half of `!=`, `<=` and `>=`.
			if i+1 < len(t) && t[i+1] == '=' {
				i += 2
				continue
			}
			if i == 0 || strings.IndexByte("!<>", t[i-1]) < 0 ||
				i >= 2 && t[i-2] == t[i-1] {
				return true
			}
			i++
		case c >= '0' && c <= '9' || strings.IndexByte(" \t\n+-*()<>!~&|^?:,", c) >= 0:
			i++
		default:
			// `/` and `%` can fail, `$` and a backquote expand, `[` reads
			// an element through a subscript of its own. Anything else is
			// counted rather than reasoned about.
			return true
		}
	}
	return false
}

// indexedAssignSubscriptText is an indexed element write's subscript, its
// words expanded into the expression the arithmetic will read — the first of
// the store's two readings of it. False where the words would not expand,
// which has been said (#5608).
//
// One function because two callers read it: the store, and the prefix walk
// that reads a prefix entry's subscript in turn ahead of the store (see
// interp/prefixsubscriptinturn.go). Two copies of this is how the walk's line
// and the store's element would come to disagree.
func (r *Runner) indexedAssignSubscriptText(a *syntax.Assign) (string, bool) {
	failedBefore := r.expansionHasFailed()
	var text string
	if r.indexedSubscriptKeepsItsQuoting(a.IndexText, a.Name) {
		text = r.expandArithText(a.IndexText, arithTextWritten)
	} else {
		endText := r.readingQuotedText()
		text = r.joinWord(a.Index)
		endText()
	}
	if r.expansionFailedHere(failedBefore) {
		// See the same stop in subscriptOver (#5608).
		return "", false
	}
	return text, true
}

// indexedAssignPosition is the second reading: the expression evaluated to a
// position, after the value. A subscript that will not evaluate is reported
// in the words the identical text inside `$(( ))` earns, and is false.
//
// Told what the source spelled as well as what it came to, because that is
// what decides whether a comma still in the expanded text separates anything:
// measured, `a=(p q r s); i="1,2"; a[$i]=Z` writes the *first* element in the
// shell with ranges, where a written `a[2,3]+=(x)` reads the comma as the
// operator it is (#2160).
func (r *Runner) indexedAssignPosition(a *syntax.Assign, text string) (int, bool) {
	idx, err := r.subscriptValueAsWritten(subscriptSubject(r.writtenIndex(a), text), text)
	if err != nil {
		r.failedSubscript("%s\n", r.subscriptFailure(text, err))
		return 0, false
	}
	return idx, true
}
