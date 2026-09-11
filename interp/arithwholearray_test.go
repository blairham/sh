// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `*` or `@` subscript inside an arithmetic expression: the slice the same
// subscript takes in an expansion, or an ordinary subscript that is not a
// number.

func arithSlice(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := CoreSemantics()
		if r.Semantics != nil {
			s = *r.Semantics
		}
		s.ArithWholeArraySubscriptIsTheSlice = a
		r.Semantics = &s
	}
}

func TestAWholeArraySubscriptInAnExpressionAnswersByAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, slice string }{
		{"a table under `*`", `typeset -A m; m[k]=9; echo $(( m[*] ))`, "9"},
		{"a table under `@`", `typeset -A m; m[k]=9; echo $(( m[@] ))`, "9"},
		{"an array of one", `a=(3); echo $(( a[*] + 1 ))`, "4"},
		{"an empty array", `a=(); echo $(( a[*] ))`, "0"},
		{"a scalar", `s=7; echo $(( s[*] ))`, "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, subscripting, arithSlice(Yes))
			if got := strings.TrimSpace(out); got != tc.slice || status != 0 {
				t.Errorf("the slice: %s = %q (status %d), want %q at 0", tc.src, got, status, tc.slice)
			}
			// The other answer reads the brackets as a subscript, where `*`
			// is a key an association has not got and no number at all on an
			// array — so the row is not the slice, whatever else it is.
			out, _ = runGrammar(t, tc.src, subscripting, arithSlice(No))
			if got := strings.TrimSpace(out); got == tc.slice && tc.slice != "0" {
				t.Errorf("the subscript reading: %s = %q, want anything but the slice", tc.src, got)
			}
		})
	}
}

// The join is the first character of IFS, and `@` joins the same way `*` does
// — there is no field splitting inside an expression for the two to part
// over, which is what keeps this off the unquoted-`@` axis.
func TestTheArithmeticSliceJoinsOnIFS(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(3 4); IFS=; echo $(( a[*] ))`, "34"},
		{`a=(3 4); IFS=; echo $(( a[@] ))`, "34"},
	} {
		out, status := runGrammar(t, tc.src, subscripting, arithSlice(Yes))
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
		}
	}
}

// A slice of more than one element is usually a failure rather than a number,
// which is the answer and not a defect in it: the joined text is read as an
// expression, and `3 4 5` is not one.
func TestASliceOfSeveralElementsFailsAsAnExpression(t *testing.T) {
	out, status := runGrammar(t, `a=(3 4 5); echo $(( a[*] ))`, subscripting, arithSlice(Yes))
	if status == 0 || strings.TrimSpace(out) == "3" {
		t.Errorf("a=(3 4 5); $(( a[*] )) = %q (status %d), want the expression to fail", out, status)
	}
}

// The slice is read *before* the association, because the key `*` is exactly
// what the other answer makes of the same text: a table consulted first would
// answer the key and the two readings would collapse into one.
func TestTheSliceIsAskedBeforeTheKey(t *testing.T) {
	const src = `typeset -A m; m[k]=9; m[*]=4; echo $(( m[*] ))`
	out, _ := runGrammar(t, src, subscripting, arithSlice(No))
	if got := strings.TrimSpace(out); got != "4" {
		t.Errorf("the subscript reading: %s = %q, want the key's value", src, got)
	}
	out, _ = runGrammar(t, src, subscripting, arithSlice(Yes))
	if got := strings.TrimSpace(out); got == "4" {
		t.Errorf("the slice: %s = %q, want the slice rather than the key", src, got)
	}
}
