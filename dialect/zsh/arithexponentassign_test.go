// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// `**=`, exponentiation's compound assignment. This shell has it and the
// three other columns that parse `**` at all refuse it — see
// dialect/bash/arithexponentassign_test.go and the ksh one beside it, which
// are the controls that make this a dialect flag rather than an operator the
// core hands everyone.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f` with no startup files. `go version
// -m` says *not a Go executable* for it. See
// syntax.Dialect.ArithExponentAssign (#4663).
func TestTheExponentAssignment(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "the issue's row",
			src:  `integer n=2; print -- "$(( n **= 3 )) $n"`,
			want: "8 8\n",
		},
		{
			// An unset name is `0 ** 3`, which is 0 rather than a refusal.
			name: "an unset name",
			src:  `print -- "$(( m **= 3 ))"; typeset -p m`,
			want: "0\ntypeset -i m=0\n",
		},
		{
			name: "a name with no numeric attribute keeps none",
			src:  `x=2; print -- "$(( x **= 3 ))"; typeset -p x`,
			want: "8\ntypeset x=8\n",
		},
		{
			name: "a float-attributed name keeps its letter and its places",
			src:  `float q=2; print -- "$(( q **= 0.5 ))"; typeset -p q`,
			want: "1.4142135623730951\ntypeset -E q=1.414213562e+00\n",
		},
		{
			name: "and an integer-attributed one keeps the integer",
			src:  `integer i=2; print -- "$(( i **= 3 ))"; typeset -p i`,
			want: "8\ntypeset -i i=8\n",
		},

		// **The exponent is a float operation, and the name's own attribute
		// is what truncates.** The *value* of the assignment is the float
		// either way — which is the half a probe that only reads the name
		// back cannot see, and the rule numericAttribute already carries for
		// every other compound operator.
		{
			name: "a negative exponent on an integer name",
			src:  `integer n=2; print -- "$(( n **= -1 )) $n"; typeset -p n`,
			want: "0.5 0\ntypeset -i n=0\n",
		},
		{
			name: "a fractional exponent on an integer name",
			src:  `integer n=4; print -- "$(( n **= 0.5 )) $n"; typeset -p n`,
			want: "2. 2\ntypeset -i n=2\n",
		},
		{
			name: "and a float name takes the float",
			src:  `float f=2; print -- "$(( f **= 3 ))"; typeset -p f`,
			want: "8.\ntypeset -E f=8.000000000e+00\n",
		},

		// Right-associative, which `**=` inherits from `**`: the second
		// assignment is read as the first one's value, so `b` is 9 and `a` is
		// `2 ** 9`.
		{
			name: "two of them in a row are read rightwards",
			src:  `integer a=2 b=3; print -- "$(( a **= b **= 2 )) $a $b"`,
			want: "512 512 9\n",
		},

		{
			name: "an element of an array",
			src:  `a=(1 2 3); print -- "$(( a[2] **= 3 ))"; print -- "$a"`,
			want: "8\n1 8 3\n",
		},

		// Every construct that assigns inside arithmetic takes it, which is
		// what says the operator was added to the grammar rather than to one
		// expansion.
		{"through let", `integer n=2; let "n **= 3"; print -- "$n"`, "8\n"},
		{"through the older spelling", `integer n=2; print -- "$[ n **= 3 ] $n"`, "8 8\n"},
		{
			name: "through a C-style for header",
			src:  `integer n=2; for (( ; n < 100; n **= 2 )); do print -- "$n"; done`,
			want: "2\n4\n16\n",
		},

		// Neither the math module nor the other precedence order moves it.
		// `zmodload` is what puts the extra functions in reach of real zsh;
		// `c_precedences` moves `**` among the binary rungs and leaves the
		// assignment where every assignment is, looser than all of them.
		{
			name: "with the math module loaded",
			src:  `zmodload zsh/mathfunc; integer n=2; print -- "$(( n **= 3 )) $n"`,
			want: "8 8\n",
		},
		{
			name: "under the other precedence order",
			src:  `setopt c_precedences; integer n=2; print -- "$(( n **= 3 )) $n"`,
			want: "8 8\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// **What `**=` declares on a name that does not exist is keyed on the value's
// type and not on the operator**, which is the rule #4605 arrived at for `=`
// and `+=` — and this operator is the one that could have parted from them,
// because `**` is a float operation. It does not.
//
// The two pairs below are built to hold one noun fixed at a time. The first
// holds the *operator* fixed and moves the value's type: `2` declares an
// integer and `2.0` a float. The second holds the *value* fixed at an integer
// and moves the operator across all three spellings: the answer does not
// move. So a reading of "`**` produces a float, so `**=` declares a float"
// is refuted by the first row, and "the operator decides" by the second
// three.
//
// The type is read through `${(t)w}`, which names the attribute, rather than
// through the attribute's own rendering — see
// dialect/zsh/arithdeclaredtype_test.go for why that route is the one that
// can tell the states apart.
func TestWhatTheExponentAssignmentDeclaresIsTheValuesType(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		// The operator held fixed, the value's type moving.
		{"an integer value", `print -- "$(( w **= 2 )) ${(t)w}"`, "0 integer\n"},
		{"a float value", `print -- "$(( w **= 2.0 )) ${(t)w}"`, "0. float\n"},
		// The value's type held fixed, the operator moving over all three.
		{"the same value through `=`", `print -- "$(( w = 2 )) ${(t)w}"`, "2 integer\n"},
		{"the same value through `+=`", `print -- "$(( w += 2 )) ${(t)w}"`, "2 integer\n"},
		{"and through `**=`", `print -- "$(( w **= 2 )) ${(t)w}"`, "0 integer\n"},
		// The listing the issue's table is written in, beside the route that
		// does not go through it.
		{"the float listing", `print -- "$(( w **= 2.0 ))"; typeset -p w`, "0.\ntypeset -F w=0.0000000000\n"},
		{"the integer listing", `print -- "$(( w **= 2 ))"; typeset -p w`, "0\ntypeset -i w=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The refusals the operator does not take away, and the one it changes.
//
// An assignment to something that cannot hold a value is `lvalue required`
// here — the whole operator blamed, not the `=` the exponent rung would have
// left behind — and three characters written apart are the exponent operator
// with its operand missing. Measured the same day.
func TestTheExponentAssignmentsRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a number on the left", `print -- "$(( 1 **= 2 ))"`, "bad math expression: lvalue required"},
		{"a name buried in an expression", `x=2; print -- "$(( 1 + x **= 3 ))"`, "bad math expression: lvalue required"},
		{"a space between the operator and the `=`", `print -- "$(( a ** = 3 ))"`, "operand expected at `= 3 '"},
		{"a third star", `print -- "$(( x ***= 3 ))"`, "operand expected at `*= 3 '"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if !strings.Contains(out, tc.want) || st == 0 {
				t.Errorf("%s = %q at %d, want it to contain %q at nonzero", tc.src, out, st, tc.want)
			}
		})
	}
}
