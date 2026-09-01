// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// runFloat evaluates src in a dialect that has floats, with the formatting and
// the integer-operator answers given. It names the flags rather than a shell,
// as this package's rule requires.
func runFloat(t *testing.T, src string, digits int, keepPoint bool, refuse Answer) string {
	t.Helper()
	d := syntax.Core()
	d.ArithFloat = true
	f, err := syntax.Parse(src, d)
	if err != nil {
		return "parse: " + err.Error()
	}
	var out bytes.Buffer
	s := PosixSemantics()
	s.ArithIntegerOperatorRefusesFloat = refuse
	dg := Diagnostics{
		ArithFloatDigits:           digits,
		ArithFloatKeepsPoint:       keepPoint,
		ArithInfinity:              "Inf",
		ArithNotANumber:            "NaN",
		ArithInvalidFloatOperation: "invalid floating point operation",
	}
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s, Diagnostics: &dg}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

// An expression is integer until a float enters it, which is what keeps `3/2`
// answering 1 in a shell that has floats.
func TestAFloatIsWhatMakesAnExpressionFloat(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((3/2))`, "1"},
		{`echo $((3.0/2))`, "1.5"},
		{`echo $((3/2.0))`, "1.5"},
		{`echo $((1.5))`, "1.5"},
		{`echo $((.5))`, "0.5"},
		{`echo $((1.5e2))`, "150"},
		{`echo $((-1.5))`, "-1.5"},
		{`echo $((1.5*2))`, "3"},
		// A variable's value is read the same way as a literal.
		{`x=1.5; echo $((x*2))`, "3"},
		// A comparison answers a truth whatever it compared, so it is an
		// integer even here.
		{`echo $((1.5 < 2))`, "1"},
		{`echo $((1.5 == 1.5))`, "1"},
		{`echo $((1.5 ? 3 : 4))`, "3"},
		// Based literals are integers whose digits may include an `e`.
		{`echo $((0x1e))`, "30"},
		// Dividing a float by zero is an infinity rather than the error the
		// integer division gives: there is no integer to hand back.
		{`echo $((1.0/0))`, "Inf"},
		{`echo $((-1.0/0))`, "-Inf"},
		{`echo $((0.0/0))`, "NaN"},
	} {
		if got := runFloat(t, tc.src, 17, false, No); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}

// The assignment leaves the float behind and not its truncation.
func TestAFloatAssignmentOutlivesTheExpression(t *testing.T) {
	if got, want := runFloat(t, `i=0; echo $((i+=1.5)); echo "[$i]"`, 17, false, No), "1.5\n[1.5]"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// How many digits a float shows, and whether a whole one keeps its point, are
// both the dialect's — the same arithmetic reads differently.
func TestFloatFormattingIsTheDialects(t *testing.T) {
	for _, tc := range []struct {
		name      string
		digits    int
		keepPoint bool
		src, want string
	}{
		{"seventeen digits", 17, false, `echo $((0.1+0.2))`, "0.30000000000000004"},
		{"fifteen rounds it", 15, false, `echo $((0.1+0.2))`, "0.3"},
		{"seventeen on a seventh", 17, false, `echo $((1.0/7))`, "0.14285714285714285"},
		{"fifteen on a seventh", 15, false, `echo $((1.0/7))`, "0.142857142857143"},
		{"a whole float keeps its point", 17, true, `echo $((1.5+2.5))`, "4."},
		{"or does not", 17, false, `echo $((1.5+2.5))`, "4"},
		// An exponent is already unmistakable, so no point is added to it.
		{"an exponent needs no point", 17, true, `echo $((1.0e20))`, "1e+20"},
		// Nor is one added to a name.
		{"an infinity needs no point", 17, true, `echo $((1.0/0))`, "Inf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := runFloat(t, tc.src, tc.digits, tc.keepPoint, No); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// What an operator defined on integers does with a float.
func TestArithIntegerOperatorRefusesFloatIsAnAxis(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		src  string
		want string
	}{
		// A remainder is a float operation on the tolerant side and refused
		// on the other, and either operand may be the float — `7%2.5` has a
		// whole number on the left, and asking only about that let the
		// refusal through.
		{"remainder truncates nothing", No, `echo $((7%2.5))`, "2"},
		{"remainder refused", Yes, `echo $((7%2.5))`, "invalid floating point operation"},
		{"remainder refused from the left", Yes, `echo $((7.5%2))`, "invalid floating point operation"},
		{"remainder of two floats", No, `echo $((7.5%2))`, "1.5"},
		// A bitwise operator is not the same question: the tolerant side
		// truncates rather than working in floating point.
		{"bitwise truncates", No, `echo $((1.5 & 1))`, "1"},
		{"bitwise refused", Yes, `echo $((1.5 & 1))`, "invalid floating point operation"},
		{"shift truncates", No, `echo $((1.5 << 1))`, "2"},
		{"complement truncates", No, `echo $((~1.5))`, "-2"},
		// An integer operand never raises the question.
		{"integers need no answer", Unspecified, `echo $((7%2))`, "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := runFloat(t, tc.src, 17, false, tc.a)
			if !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// Without the grammar there are no floats at all, and the same text is a bad
// operand rather than a number. One question, asked where each half needs it.
func TestWithoutTheGrammarThereAreNoFloats(t *testing.T) {
	if _, err := syntax.Parse(`echo $((1.5))`, syntax.Core()); err == nil {
		t.Error("a float literal should not parse where the dialect has none")
	}
	// And a float arriving in a variable is refused at evaluation, which is
	// the half the parser cannot answer.
	var out bytes.Buffer
	d := syntax.Core()
	f, err := syntax.Parse(`x=1.5; echo $((x))`, d)
	if err != nil {
		t.Fatal(err)
	}
	s := PosixSemantics()
	r := &Runner{Stdout: &out, Stderr: &out, Dialect: &d, Semantics: &s}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "1.5") {
		t.Errorf("got %q, want the value refused as not a number", out.String())
	}
}

// A negative result has to survive being written, which it did not.
//
// The integer writer looped while `n > 0`, so a negative number produced no
// digits and `echo $((2-7))` printed an empty line in every dialect. Nothing
// in the corpus had a negative result in it, and no other caller of the writer
// could ever pass one — the lengths, statuses and process ids it also serves
// are never below zero.
func TestANegativeResultIsWritten(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`echo $((2-7))`, "-5"},
		{`echo $((0-5))`, "-5"},
		{`echo $((-5))`, "-5"},
		{`echo $((~1))`, "-2"},
		{`x=$((3-9)); echo "[$x]"`, "[-6]"},
		{`echo $((0))`, "0"},
		{`echo $((5))`, "5"},
	} {
		if got, _ := run(t, tc.src, nil); strings.TrimSpace(got) != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, strings.TrimSpace(got), tc.want)
		}
	}
}
