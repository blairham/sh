// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An arithmetic assignment to a name that does not exist declares one, and
// **the value's type says which numeric attribute it gets**: an integer value
// declares `integer`, a float value declares `float` at the `F` letter's
// default precision.
//
// Two readings agree with that one on nearly every expression a script holds,
// and both are wrong: "a point was written in the text", and "the number did
// not come out whole". The rows below are built to part them, because a grid
// that only varies a lot of things would confirm either.
//
// **The type is read by a route that does not go through it.** `${(t)xx}`
// names the attribute; reading the value back through `typeset -p` and
// finding a float would compare the attribute's own rendering with itself, in
// exactly the way #4475 warns about. `typeset -p` is asserted beside it
// because the issue's table is written in it, not instead of it.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f` with no startup files, each row in
// a shell where the name has never existed. `go version -m` says *not a Go
// executable* for it. See declareFloatFromArithmetic in interp/arith.go, and
// dialect/zsh/integerassignvalue_test.go for #4595's neighboring rule about
// the assignment's *value*.
func TestAnArithmeticDeclarationTakesItsTypeFromTheValue(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The control, and the row that carries the finding: an integer
			// value still declares an integer, so this is not "the
			// declaration is wrong".
			name: "an integer value declares an integer",
			src:  `(( xx = 5 )); print "${(t)xx}"; typeset -p xx`,
			want: "integer\ntypeset -i xx=5\n",
		},
		{
			name: "a float value declares a float",
			src:  `(( xx = 1.5 )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=1.5000000000\n",
		},

		// **The first discriminating pair.** Both values are whole; only the
		// type differs, and the declaration follows the type. So the rule is
		// not "the number did not come out whole" — that reading makes the
		// first of these an integer.
		{
			name: "a whole float value still declares a float",
			src:  `(( xx = 1.0 )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=1.0000000000\n",
		},
		{
			name: "and a whole integer value declares an integer",
			src:  `(( xx = 3/2 )); print "${(t)xx}"; typeset -p xx`,
			want: "integer\ntypeset -i xx=1\n",
		},
		{
			// The issue's own pair of the same shape, where the division is
			// the only thing that moves.
			name: "a division that goes float",
			src:  `(( xx = 3.0/2 )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=1.5000000000\n",
		},
		{
			name: "a whole quotient of floats is still a float",
			src:  `(( xx = 2.0/2 )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=1.0000000000\n",
		},

		// **The second discriminating pair.** No point is written anywhere in
		// either expression; the two names it reads differ only in their
		// type, and the declaration moves with it. So the rule is not "a
		// point was written", which makes both of these integers.
		{
			name: "a float reached through a name, with no point written",
			src:  `float ff=2; (( xx = ff )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=2.0000000000\n",
		},
		{
			name: "and an integer through the same route",
			src:  `integer ii=3; (( xx = ii )); print "${(t)xx}"; typeset -p xx`,
			want: "integer\ntypeset -i xx=3\n",
		},

		// The same pair from the other side, with a point written in both and
		// the answers still parting. `zmodload zsh/mathfunc` is what puts
		// these functions in reach of real zsh; ours has them always.
		{
			name: "a point written and an integer value",
			src:  `zmodload zsh/mathfunc; (( xx = int(2.0) )); print "${(t)xx}"; typeset -p xx`,
			want: "integer\ntypeset -i xx=2\n",
		},
		{
			name: "no point written and a float value",
			src:  `zmodload zsh/mathfunc; (( xx = float(3) )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=3.0000000000\n",
		},

		{
			// A float too large for the machine word, which is the row that
			// says the integer attribute was never applied and then widened:
			// saturating it first writes the word's greatest value.
			name: "a float past the word",
			src:  `(( xx = 1e30 )); print "${(t)xx}"; typeset -p xx`,
			want: "float\ntypeset -F xx=1000000000000000019884624838656.0000000000\n",
		},
		{"a negative float", `(( xx = -1.5 )); typeset -p xx`, "typeset -F xx=-1.5000000000\n"},

		// **The letter is `F` and not `E`**, which is a measurement and not a
		// default: `float f` on its own lists as `typeset -E f`, so the two
		// spellings of "a float" part exactly here.
		{
			name: "the declaration writes the F letter where `float` writes E",
			src:  `float ee; (( xx = 1.5 )); typeset -p ee; typeset -p xx`,
			want: "typeset -E ee=0.000000000e+00\ntypeset -F xx=1.5000000000\n",
		},

		// **No base comes with a float.** The integer route learns one from a
		// radix the expression wrote; this one does not, and the `[#16]`
		// reaches only the expansion's own rendering.
		{
			name: "an output base with a float value leaves no base behind",
			src:  `print "$(( xx = [#16] 255.9 ))"; typeset -p xx`,
			want: "16#FF\ntypeset -F xx=255.9000000000\n",
		},
		{
			name: "and its pair, an output base with an integer value",
			src:  `print "$(( xx = [#16] 255 ))"; typeset -p xx`,
			want: "16#FF\ntypeset -i16 xx=255\n",
		},

		// Every construct that assigns inside arithmetic gives the same
		// answer, which is why the rule lives where the operator is applied.
		{"through let", `let "xx = 1.5"; typeset -p xx`, "typeset -F xx=1.5000000000\n"},
		{"through an expansion", `print -n "$(( xx = 1.5 ))"$'\n'; typeset -p xx`, "1.5\ntypeset -F xx=1.5000000000\n"},
		{
			name: "through a C-style for header",
			src:  `for (( xx = 0.5; xx < 2; xx++ )); do :; done; typeset -p xx`,
			want: "typeset -F xx=2.5000000000\n",
		},
		{
			// Its control, the same header with an integer start.
			name: "and the same header with an integer start",
			src:  `for (( xx = 0; xx < 2; xx++ )); do :; done; typeset -p xx`,
			want: "typeset -i xx=2\n",
		},

		// **The operator is not a second noun here**, which is worth a pair
		// of its own because it *is* one for #4595's rule about the value. A
		// compound assignment to a name that does not exist declares by the
		// same reading.
		{
			name: "a compound assignment declares by the value too",
			src:  `print "$(( xx += 1.5 ))"; typeset -p xx`,
			want: "1.5\ntypeset -F xx=1.5000000000\n",
		},
		{
			name: "and a compound with an integer value declares an integer",
			src:  `print "$(( xx += 1 ))"; typeset -p xx`,
			want: "1\ntypeset -i xx=1\n",
		},
		{
			name: "a step declares an integer, its value being one",
			src:  `print "$(( xx++ ))"; typeset -p xx`,
			want: "0\ntypeset -i xx=1\n",
		},
		{
			name: "a chain declares both links a float",
			src:  `print "$(( c1 = c2 = 1.5 ))"; typeset -p c1; typeset -p c2`,
			want: "1.5\ntypeset -F c1=1.5000000000\ntypeset -F c2=1.5000000000\n",
		},

		// The number behind the rendering is kept, which the `F` letter's ten
		// places cannot show on its own: a later letter reads every digit the
		// expression produced. `0.3333333333` could not produce this line.
		{
			name: "the digits past the rendering survive the declaration",
			src:  `(( xx = 1.0/3 )); typeset -E17 xx; print "$xx"`,
			want: "3.3333333333333331e-01\n",
		},
		{
			name: "and the length a child and ${#} see is the rendering",
			src:  `(( xx = 1.5 )); print "${#xx}"`,
			want: "12\n",
		},

		// The declaration reaches only a name the assignment *creates*, which
		// is the half #4595's rule stands on: the attribute is read before
		// the store, so a name that was already there converts and a name
		// this declares does not.
		{
			name: "a name that already exists is untouched",
			src:  `xx=3; (( xx = 1.5 )); print "${(t)xx}"; typeset -p xx`,
			want: "scalar\ntypeset xx=1.5\n",
		},
		{
			name: "and one declared without a type",
			src:  `typeset xx; (( xx = 1.5 )); print "${(t)xx}"; typeset -p xx`,
			want: "scalar\ntypeset xx=1.5\n",
		},
		{
			// An element is not a declaration at all: the array is made and
			// the value written as text.
			name: "an element of a name that does not exist",
			src:  `(( b[2] = 1.5 )); typeset -p b`,
			want: "typeset -a b=( '' 1.5 )\n",
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
