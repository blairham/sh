// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"slices"
	"strings"

	"github.com/blairham/sh/syntax"
)

// A declaration builtin's operand may carry the append operator: `declare
// a+=2` joins the value the name is holding rather than replacing it, exactly
// as the bare `a+=2` statement does.
//
// One shell takes it and two refuse it, so it is an axis — see
// Semantics.DeclarationTakesAnAppendOperand. The refusal is each shell's own
// and was already in hand: the name the complaint carries is the text in
// front of the `=`, which is `a+`, and not the whole operand.

// appendOperand splits a declaration operand that carries the append
// operator, answering the name it declares and whether the `+` was there.
//
// Text only — no axis is asked here, because the shape has to be recognized
// before anyone can be asked about it, and because the caller that *does* ask
// is the one that knows which builtin this is. A `+` with no `=` after it is
// not this: `declare a+` is refused as a name in every shell measured, the
// one that takes `a+=2` included, so the operator is the pair and not the
// character.
func appendOperand(operand string) (name string, appends bool) {
	before, _, hasValue := strings.Cut(operand, "=")
	if !hasValue || !strings.HasSuffix(before, "+") || len(before) == 1 {
		return "", false
	}
	return strings.TrimSuffix(before, "+"), true
}

// declarationOperand splits a declaration builtin's operand into the name it
// names, the value it carries and whether the value joins what is there.
//
// The axis is not asked here either. builtinNames has already refused the
// operand in the dialects that do not take it, so an append reaching a
// declaration loop is one this dialect said yes to — and asking twice would
// report an unanswered axis twice for one operand.
func declarationOperand(operand string) (name, value string, hasValue, appends bool) {
	return declarationOperandSplit(operand, false)
}

// declarationOperand is the free function's split, asked whether an `=`
// inside a subscript belongs to the subscript — see
// Semantics.DeclarationOperandSplitsAfterTheSubscript.
func (r *Runner) declarationOperand(operand string) (name, value string, hasValue, appends bool) {
	return declarationOperandSplit(operand, r.operandSplitsAfterTheSubscript(operand))
}

// appendOperand is the free function's, asked the same question.
func (r *Runner) appendOperand(operand string) (name string, appends bool) {
	if r.operandSplitsAfterTheSubscript(operand) {
		before, _, hasValue := cutOperandAfterTheSubscript(operand)
		if !hasValue || !strings.HasSuffix(before, "+") || len(before) == 1 {
			return "", false
		}
		return strings.TrimSuffix(before, "+"), true
	}
	return appendOperand(operand)
}

// operandSplitsAfterTheSubscript asks the axis only of an operand where the
// answer moves the split: a name, a subscript holding an `=`, and an `=`
// after it.
//
// An operand the **source** closed is split after its subscript in every
// column, and the axis is not asked: the parser saw where the key ended
// before anything was expanded, and the column that cuts text at its first
// `=` cuts only text — `typeset m[e=f]=y` is the key `e=f` in zsh 5.9.2 where
// `typeset 'm[e=f]=y'` is `not an identifier: m[e` (#6896). The word's shape
// is gone from the string the builtin holds, so it is carried from the word
// by Runner.sourceClosedSubscriptOperands.
func (r *Runner) operandSplitsAfterTheSubscript(operand string) bool {
	first, _, _ := strings.Cut(operand, "=")
	if !strings.Contains(first, "[") {
		return false
	}
	before, _, ok := cutOperandAfterTheSubscript(operand)
	if !ok || before == first {
		return false
	}
	if slices.Contains(r.sourceClosedSubscriptOperands, strings.TrimSuffix(before, "+")) {
		return true
	}
	return r.ask(r.sem().DeclarationOperandSplitsAfterTheSubscript,
		"a declaration operand's `=` looked for after its subscript")
}

// cutDeclarationOperand is strings.Cut at the `=` that ends a declaration
// operand's name, with the `+` of an append left on the name: the raw halves
// declarationOperandSplit reads, for a caller that spells the operand rather
// than performs it.
func (r *Runner) cutDeclarationOperand(operand string) (before, after string, found bool) {
	if r.operandSplitsAfterTheSubscript(operand) {
		return cutOperandAfterTheSubscript(operand)
	}
	return strings.Cut(operand, "=")
}

func declarationOperandSplit(operand string, afterTheSubscript bool) (name, value string, hasValue, appends bool) {
	if afterTheSubscript {
		name, value, hasValue = cutOperandAfterTheSubscript(operand)
	} else {
		name, value, hasValue = strings.Cut(operand, "=")
	}
	if !hasValue {
		return name, value, hasValue, false
	}
	if strings.HasSuffix(name, "+") && len(name) > 1 {
		return strings.TrimSuffix(name, "+"), value, true, true
	}
	return name, value, hasValue, false
}

// cutOperandAfterTheSubscript is strings.Cut at the `=` that follows a
// name's closing bracket, the brackets counted so that a subscript may hold
// brackets and `=` of its own: `m[a=b]=w` is the key `a=b`, `m[b[1]=2]=v`
// the key `b[1]=2`. An operand that is not a name, a bracketed subscript and
// then `=` or `+=` is cut at its first `=`, which is every other operand.
func cutOperandAfterTheSubscript(operand string) (before, after string, found bool) {
	i := 0
	for i < len(operand) && (operand[i] == '_' || operand[i] >= 'a' && operand[i] <= 'z' ||
		operand[i] >= 'A' && operand[i] <= 'Z' || i > 0 && operand[i] >= '0' && operand[i] <= '9') {
		i++
	}
	if i > 0 && i < len(operand) && operand[i] == '[' {
		depth := 0
	scan:
		for j := i; j < len(operand); j++ {
			switch operand[j] {
			case '[':
				depth++
			case ']':
				depth--
			}
			if depth != 0 {
				continue
			}
			rest := operand[j+1:]
			switch {
			case strings.HasPrefix(rest, "="):
				return operand[:j+1], rest[1:], true
			case strings.HasPrefix(rest, "+="):
				return operand[:j+2], rest[2:], true
			}
			break scan
		}
	}
	return strings.Cut(operand, "=")
}

// appendOverCompound performs `name+=value` where the name is holding an
// array or a table, and reports whether it did.
//
// The scalar join is left to the caller, because a declaration has more to do
// with the value than an assignment statement does. What the two share is
// this: which store the append reaches is decided by what the name holds, and
// a declaration that went straight to the scalar store would leave a string
// where the script built an array. Measured on bash 5.3.15, which is the
// shell that takes the operand:
//
//	declare -a arr=(p q); declare arr+=x    declare -a arr=([0]="px" [1]="q")
//	declare -A m=([k]=v); declare m+=x      declare -A m=([0]="x" [k]="v" )
//
// Both are the answers the bare `arr+=x` statement already gives, which is
// why this is the statement's own code rather than a second reading of it.
func (r *Runner) appendOverCompound(name, value string) bool {
	if r.assocDeclared(name) {
		// `m+=x` over a declared table joins the element whose key is `0`.
		v, ok := r.appendedValue(name, r.assocsAt(name)["0"].scalar(), value)
		if !ok {
			return true
		}
		r.setAssocElem(name, "0", v)
		return true
	}
	if old, ok := r.arraysLookup(name); ok {
		// Where the value joins an array is an axis, asked inside.
		r.appendScalarToArray(name, old, value)
		return true
	}
	return false
}

// appendedScalar is the value `name+=value` leaves a name holding a string.
//
// Joined through appendedValue rather than with `+`, because the name's
// attributes decide which join this is: `declare -i a=1; declare a+=2` is 3
// and not 12.
//
// A declaration that has just made a **fresh** binding has nothing to join
// to, whatever the name outside it holds: `a=1; f(){ local a+=2; }` leaves
// `2` in the local and `1` in the caller. The cell the value lands in is the
// one the append reads, which is the same rule stated once rather than a
// scope question of its own — and the declarations that take no shadow show
// the other side of it, since `export a+=2` and `declare -g a+=2` inside a
// function both leave `12`.
func (r *Runner) appendedScalar(name, value string, fresh bool) (string, bool) {
	old := ""
	if !fresh {
		// storedVar and not getVar: an append joins what the name was
		// assigned, which in one shell is not what a read of it answers —
		// and through appendedOldValue, because a self-aimed reference's
		// join must not carry the read's warning. See there.
		old = r.appendedOldValue(name)
	}
	return r.appendedValue(name, old, value)
}

// declarationAppend stores a declaration's `name+=value`, and reports whether
// it could. A refused join has already said so.
func (r *Runner) declarationAppend(name, value string, global, fresh bool) bool {
	if r.appendOverCompound(name, value) {
		return true
	}
	v, ok := r.appendedScalar(name, value, fresh)
	if !ok {
		return false
	}
	if global {
		r.setGlobalVar(name, v)
		return true
	}
	r.setVarAs(name, v, assignedByDeclaration)
	return true
}

// appendNameSplit finds the `=` of a declaration operand's `name+=`, and
// answers where it is in the same shape assignNameSplit does: the span it
// lives in and its offset within that span.
//
// The appending half of declarationNameSplit, and its own predicate beside
// assignNameSplit rather than a loosening of it: assignNameSplit is shared with
// keywordPromotable, where taking `x+` as a name would make `set -k`'s
// `x+=1 cmd` a prefix assignment *named* `x+` rather than an append, which
// nothing has measured.
//
// A subscript is taken here as it is there. `typeset a[1]+=q` was left out
// when the plain name was taken (#3772) because it is a wider divergence and
// three-part, and it is this issue's (#3789): the word was split into fields
// and then matched against the filesystem, and the element it named was
// replaced rather than joined.
func (r *Runner) appendNameSplit(w *syntax.Word) (span, off int, ok bool) {
	span, off, appends, ok := r.declarationNameSplit(w)
	return span, off, ok && appends
}

// An array literal written as a declaration's operand may carry the append
// operator too — `typeset u+=(3 4)` — and what the utility is *handed* there
// is not what it is handed for a scalar.
//
// The parser keeps the assignment apart and leaves a name in argv, so the
// operator has nowhere to be unless the name carries it. One column takes the
// operand and hands over the bare name; three leave the `+` on it and refuse
// the name that leaves behind — see
// Semantics.DeclarationTakesAnAppendingArrayOperand for the rows and for why
// it is not the scalar field asked again.
//
// Nothing here writes a refusal. The name `u+` is refused by the same reader
// that refuses it for a scalar operand, in each shell's own words and with
// each shell's own idea of how far the script gets afterwards, so a dialect
// that already reproduced `typeset "u+"` reproduces this by being handed the
// same word.

// arrayOperandName is the word an array-literal operand leaves in argv for the
// utility to read.
//
// The bare name where the dialect takes the operator, and the name with the
// operator still on it where it does not. Asked only of an appending literal:
// a plain `typeset a=(1 2)` has no operator to place and reaches the question
// with nothing to decide.
func (r *Runner) arrayOperandName(a *syntax.Assign) string {
	if !a.Append {
		return a.Name
	}
	if r.ask(r.sem().DeclarationTakesAnAppendingArrayOperand,
		"a declaration taking a `name+=( … )` operand") {
		return a.Name
	}
	return a.Name + "+"
}

// operandName is the name half of a declaration operand, the `=` looked for
// where the dialect looks for it — see Runner.declarationOperand.
func (r *Runner) operandName(operand string) string {
	name, _ := r.operandNameAndValued(operand)
	return name
}

// operandNameAndValued is operandName and whether the operand carried a value.
func (r *Runner) operandNameAndValued(operand string) (string, bool) {
	if r.operandSplitsAfterTheSubscript(operand) {
		name, _, valued := cutOperandAfterTheSubscript(operand)
		return name, valued
	}
	name, _, valued := strings.Cut(operand, "=")
	return name, valued
}
