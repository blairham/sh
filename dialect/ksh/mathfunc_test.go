// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// The math functions arithmetic calls in this shell, and the two sentences it
// refuses a call with. Here rather than in interp because the whole thing is
// reached through this dialect's tables — the `(` after a name being a call at
// all, the sixty-nine names, and the wording of each refusal — and a case
// built on a permissive base would see none of them.
//
// Every expectation below was measured on ksh93u+ 2012-08-01 (`/bin/ksh`,
// macOS 26) on 2026-09-13, under `env -i PATH=/usr/bin:/bin` with a scratch
// HOME (#2420).

func runKshMath(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// TestAMathFunctionIsCalledFromArithmetic is the table itself, and every row
// is chosen for an operand the *other* table would answer differently on —
// a row both readings agree about proves only that something was computed.
func TestAMathFunctionIsCalledFromArithmetic(t *testing.T) {
	for _, c := range []struct{ name, expr, want string }{
		// The whole float has no point here, where the other shell with a
		// table writes `2.` for the same number.
		{"a whole float", "sqrt(4)", "2"},
		{"the precision", "sqrt(2)", "1.4142135623731"},
		{"two operands", "pow(2,10)", "1024"},
		{"the modulus", "fmod(7,3)", "1"},
		// `int` is this shell's *floor*. The other shell truncates toward
		// zero and would answer -3 and 0 for these two, so a row using only
		// a positive operand would pass under either reading.
		{"int of a negative", "int(-3.7)", "-4"},
		{"int of a small negative", "int(-0.5)", "-1"},
		{"trunc beside it", "trunc(-3.7)", "-3"},
		// And its result is a float, which only a division can show: an
		// integer result would make this 3.
		{"int divides as a float", "7/int(2)", "3.5"},
		{"ilogb adds as a float", "ilogb(8)+0.5", "3.5"},
		// `abs` keeps the kind of its argument, and this is the operand
		// that says so: rounded to a float64 it would come back
		// 9.22337203685478e+18.
		{"abs of a large integer", "abs(-9223372036854775807)", "9223372036854775807"},
		{"abs of a float", "abs(-3.5)", "3.5"},
		// The edges are named rather than refused, which is not a given in
		// a shell whose arithmetic refuses a division by zero.
		{"a root with no real answer", "sqrt(-1)", "nan"},
		{"a logarithm of zero", "log(0)", "-inf"},
		{"half goes to even", "rint(2.5)", "2"},
		{"and up from the odd", "rint(3.5)", "4"},
		{"round goes away from zero", "round(-2.5)", "-3"},
		// C's fmax ignores a NaN rather than propagating it, which is where
		// Go's math.Max would answer nan for all three of these.
		{"a maximum beside a NaN", "fmax(1,sqrt(-1))", "1"},
		{"a NaN on the left", "fmax(sqrt(-1),1)", "1"},
		{"a minimum beside a NaN", "fmin(1,sqrt(-1))", "1"},
		// The order leads in the Bessel functions: this is J₂(1) and not
		// J₁(2), which is 0.576724807756873.
		{"the order leads", "jn(2,1)", "0.1149034849319"},
		// Three operands, and the only function here with three.
		{"a fused multiply-add", "fma(2,3,4)", "10"},
		// The predicates answer with a number like everything else.
		{"a NaN is named", "isnan(sqrt(-1))", "1"},
		{"and a number is not", "isnan(0)", "0"},
		{"an ordered pair", "isunordered(1,2)", "0"},
		{"and an unordered one", "isunordered(sqrt(-1),1)", "1"},
		// A name is still a variable where no `(` follows it, so the table
		// does not shadow anything a script writes.
		{"a name without a call", "sqrt", "0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshMath(t, "echo $(( "+c.expr+" ))\n")
			if st != 0 || out != c.want+"\n" {
				t.Fatalf("$(( %s )): status %d, output %q, want %q", c.expr, st, out, c.want+"\n")
			}
		})
	}
}

// TestAnUnknownMathFunctionIsBlamedToTheEndOfTheExpression is the first of the
// two sentences, and its extent is the whole point: the text runs from the
// first byte of the name to the end of the expression, so what stands in front
// of the call is cut off and the blank before the `))` is kept.
//
// A row written as `$(( nosuchmf(1) ))` alone cannot tell that reading from
// two others that would pass it — naming the call as written, and naming the
// whole expression — so every row here has something on at least one side of
// the call.
func TestAnUnknownMathFunctionIsBlamedToTheEndOfTheExpression(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the expression is the call",
			"echo $(( nosuchmf(1) ))",
			"ksh: nosuchmf(1) : unknown function\n",
		},
		{
			"what precedes the call is cut off",
			"echo $(( 1 + nosuchmf(1) ))",
			"ksh: nosuchmf(1) : unknown function\n",
		},
		{
			"and what follows it is kept",
			"echo $(( 1 + nosuchmf(1) + 2 ))",
			"ksh: nosuchmf(1) + 2 : unknown function\n",
		},
		{
			"blanks are the source's own",
			"echo $(( 2*nosuchmf(1)+3 ))",
			"ksh: nosuchmf(1)+3 : unknown function\n",
		},
		{
			"an expression with no blanks has none",
			"echo $((nosuchmf(1)))",
			"ksh: nosuchmf(1): unknown function\n",
		},
		{
			"a branch of a conditional reaches its colon",
			"echo $(( 1 ? nosuchmf(1) : 2 ))",
			"ksh: nosuchmf(1) : 2 : unknown function\n",
		},
		{
			// A subscript is parsed as an expression of its own, so the
			// tail stops at its close bracket rather than running on
			// through the outer expression.
			"a subscript is its own expression",
			"echo $(( x[nosuchmf(1)] ))",
			"ksh: nosuchmf(1): unknown function\n",
		},
		{
			// The name is looked up before the argument list is weighed,
			// which is what keeps an empty list from becoming the syntax
			// error a *known* name's empty list is.
			"an empty argument list is still unknown",
			"echo $(( nosuchmf() ))",
			"ksh: nosuchmf() : unknown function\n",
		},
		{
			// And before the arguments are evaluated: the division never
			// happens.
			"an argument that would fail is never reached",
			"echo $(( nosuchmf(1/0) ))",
			"ksh: nosuchmf(1/0) : unknown function\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshMath(t, c.src+"\n")
			if st == 0 || out != c.want {
				t.Fatalf("%s: status %d, output %q, want %q", c.src, st, out, c.want)
			}
		})
	}
}

// TestAMathFunctionCalledWithTheWrongCountBlamesTheWholeExpression is the
// second sentence, and it blames the *other* extent — the expression entire,
// leading blank and all, where the sentence above cuts the leading text off.
// The pair is why neither can be written from the other.
func TestAMathFunctionCalledWithTheWrongCountBlamesTheWholeExpression(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			// The leading blank `$(( ` put there is part of the blamed
			// text, which the call as written does not have.
			"the blanks the expansion put there",
			"echo $(( atan(1,2) ))",
			"ksh:  atan(1,2) : function has wrong number of arguments\n",
		},
		{
			"and none where there were none",
			"echo $((atan(1,2)))",
			"ksh: atan(1,2): function has wrong number of arguments\n",
		},
		{
			"what surrounds the call is kept",
			"echo $(( 1 + atan(1,2) + 2 ))",
			"ksh:  1 + atan(1,2) + 2 : function has wrong number of arguments\n",
		},
		{
			"too few is the same sentence",
			"echo $(( pow(1) ))",
			"ksh:  pow(1) : function has wrong number of arguments\n",
		},
		{
			"and too many",
			"echo $(( sqrt(1,2) ))",
			"ksh:  sqrt(1,2) : function has wrong number of arguments\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshMath(t, c.src+"\n")
			if st == 0 || out != c.want {
				t.Fatalf("%s: status %d, output %q, want %q", c.src, st, out, c.want)
			}
		})
	}
}

// TestAKnownMathFunctionWithNoArgumentIsASyntaxError is the byte that parts
// the two sentences from the ordinary one.
//
// `sqrt()` and `nosuchmf()` differ only in whether the shell knows the name,
// and they are worded differently for it: the known name has an operand
// missing, the unknown one is unknown. Both are refusals at the same status,
// so only the wording tells them apart — and the third row is the reminder
// that everything else a call fails on goes the ordinary way.
func TestAKnownMathFunctionWithNoArgumentIsASyntaxError(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a known name with nothing to work on",
			"echo $(( sqrt() ))",
			"ksh:  sqrt() : arithmetic syntax error\n",
		},
		{
			"where an unknown one is still unknown",
			"echo $(( sqrt2() ))",
			"ksh: sqrt2() : unknown function\n",
		},
		{
			// The argument's own failure is that failure and not the
			// function's, and it is blamed the way every other division by
			// zero here is: on the whole expression.
			"an argument that will not evaluate",
			"echo $(( sqrt(1/0) ))",
			"ksh:  sqrt(1/0) : divide by zero\n",
		},
		{
			// A blank between the name and the `(` is not a call, so the
			// grammar leaves it exactly where it was.
			"a blank before the parenthesis is no call",
			"echo $(( nosuchmf (1) ))",
			"ksh:  nosuchmf (1) : arithmetic syntax error\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKshMath(t, c.src+"\n")
			if st == 0 || out != c.want {
				t.Fatalf("%s: status %d, output %q, want %q", c.src, st, out, c.want)
			}
		})
	}
}
