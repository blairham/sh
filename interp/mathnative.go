// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"

	"github.com/blairham/sh/syntax"
)

// Math functions written in Go rather than in shell.
//
// mathfunc.go is the other half: a name arithmetic can call, backed by a shell
// function a script wrote. This half is the same registry entered from the
// other side — a name arithmetic can call, backed by code in this program —
// and it exists because one shell ships a module whose entire content is the
// C math library under such names. See dialect/zsh, which is where the names
// `sqrt`, `floor` and the other forty-four are spelled; nothing here knows any
// of them.
//
// # Why the operands are handed over unevaluated
//
// Every function in that module but one takes numbers, and handing a native
// implementation a slice of numbers would have been the smaller seam. The one
// that does not is the reason this is a call object instead: its operand is
// the *name* of a variable holding a generator's state, and evaluating that
// name as arithmetic is not merely unnecessary, it fails — the state is a
// string of hexadecimal digits, which is not a number this shell reads.
//
// So [MathCall] evaluates on demand and each implementation asks for what it
// wants. A function of numbers calls Value and never sees the difference; the
// one that wants a name calls Name and the operand is never evaluated at all.
// The alternative — a second registration kind for the one function — would
// have put the exception in the registry rather than in the function that has
// it.
//
// # The arity check is the registry's, not the function's
//
// min and max are checked in evalMathFunc, before the implementation runs and
// in exactly the same place a shell-backed registration is checked, so the
// wrong-argument-count sentence is one sentence with one wording. An
// implementation is therefore entitled to assume its operand count is in
// range, which is why the ones below index without asking.

// MathValue is a number crossing between the arithmetic evaluator and a math
// function implemented in Go.
//
// The distinction it carries is the one arithmetic itself carries: a value is
// an integer or it is a float, and which it is survives the call. That is not
// bookkeeping — measured against the module this exists for, `abs(-2)` is `2`
// and `abs(-2.0)` is `2.`, so an implementation that took and returned float64
// would have no way to say the first one.
type MathValue struct{ n arithNum }

// MathInt is an integer result.
func MathInt(i int) MathValue { return MathValue{n: intNum(i)} }

// MathFloat is a floating-point result. A whole one still reads as a float,
// which is the dialect's spelling question and is answered in formatNum.
func MathFloat(f float64) MathValue { return MathValue{n: floatNum(f)} }

// IsFloat reports whether the value is a float rather than an integer.
func (v MathValue) IsFloat() bool { return v.n.float }

// Float is the value as a float, whichever it is.
func (v MathValue) Float() float64 { return v.n.asFloat() }

// Int is the value truncated to an integer, whichever it is.
func (v MathValue) Int() int { return v.n.asInt() }

// MathCall is one call's operands, evaluated when an implementation asks for
// them and not before.
type MathCall struct {
	r    *Runner
	args []syntax.ArithExpr
}

// Len is how many operands were written.
func (c MathCall) Len() int { return len(c.args) }

// Value is one operand evaluated as arithmetic.
func (c MathCall) Value(i int) (MathValue, error) {
	n, err := c.r.evalNum(c.args[i])
	if err != nil {
		return MathValue{}, err
	}
	return MathValue{n: n}, nil
}

// Name is one operand as a bare variable name, and whether it was written as
// one. Nothing is evaluated: the operand `seed` is the name `seed` here
// whatever the variable holds, and `seed+1` is not a name at all.
func (c MathCall) Name(i int) (string, bool) {
	v, ok := c.args[i].(*syntax.ArithVar)
	if !ok {
		return "", false
	}
	return v.Name, true
}

// MathFunction is a math function implemented in Go: the operands in, one
// value out, and an error that becomes the expression's own failure with the
// error's text as its sentence.
type MathFunction func(r *Runner, call MathCall) (MathValue, error)

// RegisterMathFunction records a math function implemented in Go under a name
// arithmetic may call, taking between min and max operands — with a max of
// [MathUnbounded] for no upper limit.
//
// Deliberately **not** in the listing a shell-backed registration appears in:
// measured against the shell that has both, loading the module that brings
// forty-six of these leaves `functions -M` printing nothing at all. So the
// order that listing walks is not written here, which is the whole of what
// keeps them apart.
func (r *Runner) RegisterMathFunction(name string, minArgs, maxArgs int, fn MathFunction) {
	if r.mathFuncs == nil {
		r.mathFuncs = map[string]mathFunc{}
	}
	r.mathFuncs[name] = mathFunc{min: minArgs, max: maxArgs, impl: name, native: fn}
}

// KnownMathFunction reports whether arithmetic can call a name — registered
// natively here or by a script.
//
// The question a module gate asks about a math function, and it is asked of
// the runner rather than of a list so that the answer moves when the shell
// does. See dialect/zsh's zmodload.go, which is the caller.
func (r *Runner) KnownMathFunction(name string) bool {
	_, ok := r.mathFuncs[name]
	return ok
}

// MathUnbounded is the maximum operand count that means "as many as are
// written".
const MathUnbounded = mathFuncUnbounded

// evalNativeMathFunc runs a natively registered implementation.
//
// The arity has already been checked by the caller, which is the same check a
// shell-backed registration gets and in the same place. An error's text is the
// sentence the expression fails with, complete because an implementation that
// refuses has already said which function refused and why.
func (r *Runner) evalNativeMathFunc(fn mathFunc, x *syntax.ArithCall) (arithNum, error) {
	v, err := fn.native(r, MathCall{r: r, args: x.Args})
	if err != nil {
		var aerr arithError
		if errors.As(err, &aerr) {
			// An operand's own failure — a division by zero inside
			// `sqrt(1/0)` — is that failure and not this function's.
			return intNum(0), aerr
		}
		return intNum(0), arithError{msg: err.Error(), complete: true}
	}
	return v.n, nil
}
