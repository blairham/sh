// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Math functions written in Go: the seam a dialect registers a name arithmetic
// can call through, with the implementation in this program rather than in
// shell.
//
// The cases below name the *capability* and never a shell, as this package's
// rule requires. What the names are, what each one computes and which shell
// ships them is dialect/zsh's business.

// runNative evaluates src in a dialect with floats, after handing the runner
// the registrations given. It returns everything the shell wrote.
func runNative(t *testing.T, src string, register func(*Runner)) string {
	t.Helper()
	d := syntax.Core()
	d.ArithFloat = true
	d.ArithFunctionCall = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out bytes.Buffer
	s := PosixSemantics()
	dg := Diagnostics{
		ArithFloatDigits: 17, ArithFloatKeepsPoint: true,
		ArithInfinity: "Inf", ArithNotANumber: "NaN",
	}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg,
	})
	register(r)
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// TestANativeRegistrationKeepsWhetherItsValueIsAFloat is the reason the seam
// carries a [MathValue] rather than a float64.
//
// The two spellings are the dialect's — seventeen digits and a trailing point
// on a whole float are set above — and the *choice* between them is the
// implementation's. A seam that handed back one number type could not say
// both, so the same argument through two registrations would print the same
// text, and the two lines here would be one.
func TestANativeRegistrationKeepsWhetherItsValueIsAFloat(t *testing.T) {
	got := runNative(t, "echo $(( whole(2) )) $(( count(2) ))", func(r *Runner) {
		r.RegisterMathFunction("whole", 1, 1, func(_ *Runner, c MathCall) (MathValue, error) {
			v, err := c.Value(0)
			return MathFloat(v.Float()), err
		})
		r.RegisterMathFunction("count", 1, 1, func(_ *Runner, c MathCall) (MathValue, error) {
			v, err := c.Value(0)
			return MathInt(v.Int()), err
		})
	})
	if want := "2. 2"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAnOperandIsReadableAsANameWithoutBeingEvaluated is the whole reason
// [MathCall] hands the operands over unevaluated.
//
// One registration reads its operand as a name and the other evaluates it, and
// the variable holds text no arithmetic could read. The name-reading one
// answers; the evaluating one fails on the same line, which is what says the
// first is not simply getting lucky.
func TestAnOperandIsReadableAsANameWithoutBeingEvaluated(t *testing.T) {
	register := func(r *Runner) {
		r.RegisterMathFunction("named", 1, 1, func(_ *Runner, c MathCall) (MathValue, error) {
			name, ok := c.Name(0)
			if !ok {
				return MathInt(-1), nil
			}
			return MathInt(len(name)), nil
		})
		r.RegisterMathFunction("valued", 1, 1, func(_ *Runner, c MathCall) (MathValue, error) {
			v, err := c.Value(0)
			return MathInt(v.Int()), err
		})
	}
	if got, want := runNative(t, "state=7017f975d381; echo $(( named(state) ))", register), "5"; got != want {
		t.Errorf("reading the operand as a name = %q, want %q", got, want)
	}
	got := runNative(t, "state=7017f975d381; echo $(( valued(state) ))", register)
	if !strings.Contains(got, "7017f975d381") {
		t.Errorf("evaluating the same operand = %q, want a complaint naming the value", got)
	}
	// An operand that is not a bare name is not one, whatever it evaluates to.
	if got, want := runNative(t, "echo $(( named(1+1) ))", register), "-1"; got != want {
		t.Errorf("an expression operand = %q, want %q", got, want)
	}
}

// TestTheRegisteredOperandCountIsCheckedBeforeTheImplementationRuns.
//
// One check, in the place a shell-written registration is checked, so an
// implementation may index its operands without asking. The counter proves the
// order: a refused call must not have run the body.
func TestTheRegisteredOperandCountIsCheckedBeforeTheImplementationRuns(t *testing.T) {
	calls := 0
	register := func(r *Runner) {
		r.RegisterMathFunction("two", 2, 2, func(_ *Runner, c MathCall) (MathValue, error) {
			calls++
			a, err := c.Value(0)
			if err != nil {
				return MathValue{}, err
			}
			b, err := c.Value(1)
			if err != nil {
				return MathValue{}, err
			}
			return MathInt(a.Int() + b.Int()), nil
		})
	}
	if got, want := runNative(t, "echo $(( two(1,2) ))", register), "3"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
	if got := runNative(t, "echo $(( two(1) ))", register); !strings.Contains(got, "two(1)") {
		t.Errorf("output = %q, want a complaint quoting the call", got)
	}
	if calls != 1 {
		t.Errorf("the implementation ran %d times, want 1 — the refused call must not reach it", calls)
	}
}

// TestAnUnboundedRegistrationTakesAsManyOperandsAsAreWritten, which is what
// [MathUnbounded] says and is the same value the shell-written registrations
// already used for it.
func TestAnUnboundedRegistrationTakesAsManyOperandsAsAreWritten(t *testing.T) {
	register := func(r *Runner) {
		r.RegisterMathFunction("count", 0, MathUnbounded, func(_ *Runner, c MathCall) (MathValue, error) {
			return MathInt(c.Len()), nil
		})
	}
	got := runNative(t, "echo $(( count() )) $(( count(1) )) $(( count(1,2,3) ))", register)
	if want := "0 1 3"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAnImplementationsErrorIsTheExpressionsOwnComplaint: an implementation
// that refuses says so in its own words, and the sentence is the whole
// diagnostic rather than being wrapped in one.
func TestAnImplementationsErrorIsTheExpressionsOwnComplaint(t *testing.T) {
	got := runNative(t, "echo $(( refuse(1) ))", func(r *Runner) {
		r.RegisterMathFunction("refuse", 1, 1, func(*Runner, MathCall) (MathValue, error) {
			return MathValue{}, errors.New("refuse: nothing to compute")
		})
	})
	if want := "refuse: nothing to compute"; !strings.HasSuffix(got, want) {
		t.Errorf("output = %q, want it to end with %q", got, want)
	}
}

// TestARegisteredNameIsAskedAboutRatherThanListed.
//
// [Runner.KnownMathFunction] is the question a module gate asks about a name
// arithmetic can call, and asking the runner is what makes the answer move
// when the shell does rather than when somebody remembers to edit a list.
//
// The other half — that a registration made here stays out of the listing a
// script's own registrations are written back into — is only reachable through
// the builtin that spells them, so it is asserted where that builtin is, in
// dialect/zsh.
func TestARegisteredNameIsAskedAboutRatherThanListed(t *testing.T) {
	d := syntax.Core()
	r := newTestRunner(t, &Runner{Dialect: &d})
	if r.KnownMathFunction("native") {
		t.Error("KnownMathFunction said yes before anything was registered")
	}
	r.RegisterMathFunction("native", 0, 0, func(*Runner, MathCall) (MathValue, error) {
		return MathInt(0), nil
	})
	if !r.KnownMathFunction("native") {
		t.Error("KnownMathFunction said no for a name just registered")
	}
}
