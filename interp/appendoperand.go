// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

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
	name, value, hasValue = strings.Cut(operand, "=")
	if !hasValue {
		return name, value, hasValue, false
	}
	if base, ok := appendOperand(operand); ok {
		return base, value, true, true
	}
	return name, value, hasValue, false
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
		v, ok := r.appendedValue(name, r.AssocArrays[name]["0"], value)
		if !ok {
			return true
		}
		r.setAssocElem(name, "0", v)
		return true
	}
	if old, ok := r.Arrays[name]; ok {
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
		// assigned, which in one shell is not what a read of it answers.
		old, _ = r.storedVar(name)
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
