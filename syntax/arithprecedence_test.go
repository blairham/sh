// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// The other ladder: Dialect.ArithPrecedence at
// ArithPrecedenceShiftsAndBitwiseBindTighter. Tests name the flag, never a
// shell — docs/spec/grammar/arithmetic.md holds the measurement and which
// dialect carries which value.
//
// Every row is a tree rather than a number, which is the whole of what this
// flag decides: nothing about what an operator *means* changes, only which
// operands it is given.

func shiftsAndBitwiseFirst() Dialect {
	d := Core()
	d.ArithPrecedence = ArithPrecedenceShiftsAndBitwiseBindTighter
	return d
}

func TestTheShiftsAndTheBitwiseOperatorsCanBindTighter(t *testing.T) {
	for _, tc := range []struct{ src, c, moved string }{
		// The shifts, against both rungs they move above.
		{`1<<2+1`, `(1 << (2 + 1))`, `((1 << 2) + 1)`},
		{`1+2<<1`, `((1 + 2) << 1)`, `(1 + (2 << 1))`},
		{`1<<2*2`, `(1 << (2 * 2))`, `((1 << 2) * 2)`},
		{`16>>1+1`, `(16 >> (1 + 1))`, `((16 >> 1) + 1)`},
		// The bitwise operators, against the comparisons and against `+`.
		{`1<2&1`, `((1 < 2) & 1)`, `(1 < (2 & 1))`},
		{`6|1+1`, `(6 | (1 + 1))`, `((6 | 1) + 1)`},
		{`1==2^3`, `((1 == 2) ^ 3)`, `(1 == (2 ^ 3))`},
		// And against `**`, which is the rung a reading of the shift rows
		// alone would miss: the bitwise operators bind tighter than
		// exponentiation under the moved ladder and looser under C's.
		{`2**1|3`, `((2 ** 1) | 3)`, `(2 ** (1 | 3))`},
		{`2|1**3`, `(2 | (1 ** 3))`, `((2 | 1) ** 3)`},
		// The three bitwise operators keep their order relative to each
		// other, which is what makes this two rungs moving rather than a
		// different ladder.
		{`1|2^3&4`, `(1 | (2 ^ (3 & 4)))`, `(1 | (2 ^ (3 & 4)))`},
		// And so does everything from `*` down.
		{`1+2*3`, `(1 + (2 * 3))`, `(1 + (2 * 3))`},
		{`1||2&&3`, `(1 || (2 && 3))`, `(1 || (2 && 3))`},
		{`1<2==3`, `((1 < 2) == 3)`, `((1 < 2) == 3)`},
	} {
		t.Run(tc.src, func(t *testing.T) {
			if got := arith(parseArithOf(t, tc.src, Core())); got != tc.c {
				t.Errorf("as in C: got %s, want %s", got, tc.c)
			}
			if got := arith(parseArithOf(t, tc.src, shiftsAndBitwiseFirst())); got != tc.moved {
				t.Errorf("shifts and bitwise first: got %s, want %s", got, tc.moved)
			}
		})
	}
}

// Parentheses are what say this is precedence and not a broken operator: told
// which grouping to use, the two ladders build the same tree.
func TestParenthesesSettleBothLadders(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`1<<(2+1)`, `(1 << (2 + 1))`},
		{`(1+2)<<1`, `((1 + 2) << 1)`},
		{`(1<2)&1`, `((1 < 2) & 1)`},
		{`(2**1)|3`, `((2 ** 1) | 3)`},
	} {
		for _, d := range []Dialect{Core(), shiftsAndBitwiseFirst()} {
			if got := arith(parseArithOf(t, tc.src, d)); got != tc.want {
				t.Errorf("%s under %v: got %s, want %s", tc.src, d.ArithPrecedence, got, tc.want)
			}
		}
	}
}

// Associativity does not move with the rungs. `**` is right-associative and
// the rest left-associative under both orders, which is what keeps the moved
// ladder a reordering rather than a second grammar.
func TestTheLadderMovesWithoutMovingAssociativity(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`2**3**2`, `(2 ** (3 ** 2))`},
		{`1+2-3`, `((1 + 2) - 3)`},
		{`8>>1>>1`, `((8 >> 1) >> 1)`},
	} {
		for _, d := range []Dialect{Core(), shiftsAndBitwiseFirst()} {
			if got := arith(parseArithOf(t, tc.src, d)); got != tc.want {
				t.Errorf("%s under %v: got %s, want %s", tc.src, d.ArithPrecedence, got, tc.want)
			}
		}
	}
}

// A dialect without `**` reads the moved ladder's exponent rung as no rung at
// all, so the operands come from the one below it and nothing is refused for
// the operator being absent.
func TestTheExponentRungIsSkippedWhereTheDialectHasNoExponent(t *testing.T) {
	d := shiftsAndBitwiseFirst()
	d.ArithExponent = false
	if got := arith(parseArithOf(t, `6|1+1`, d)); got != `((6 | 1) + 1)` {
		t.Errorf("got %s, want ((6 | 1) + 1)", got)
	}
}
