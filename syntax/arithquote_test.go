// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax

import "testing"

// A double quote inside an arithmetic expression is read three ways, and the
// flag is tested by policy and never by shell (#1223).
//
// It is worth the three: a quote between two operands is answered alike by
// four of the panel, and a quote *inside* a token separates them again — one
// takes the bytes out of the text before reading it, so `1"0"` is the number
// ten, and two step over the byte where a token may begin, so the same text is
// two operands running together.
func TestADoubleQuoteInArithmeticByPolicy(t *testing.T) {
	quoted := func(p ArithDoubleQuotePolicy) Dialect {
		d := Core()
		d.ArithDoubleQuote = p
		return d
	}
	// An expression the reader will not take leaves nil behind rather than
	// failing the file: the failure is carried to run time, because the text
	// an expression is read from is not final until its parameters have gone
	// in. So nil here is "not accepted".
	for _, tc := range []struct {
		name string
		p    ArithDoubleQuotePolicy
		src  string
		read bool
	}{
		{"refused: a quoted operand is no operand", ArithDoubleQuoteRefused, `"1" + 1`, false},
		{"skipped: a quoted operand reads through", ArithDoubleQuoteSkipped, `"1" + 1`, true},
		{"removed: a quoted operand reads through", ArithDoubleQuoteRemoved, `"1" + 1`, true},
		{"skipped: a quote inside a token ends it", ArithDoubleQuoteSkipped, `1"0"`, false},
		{"removed: a quote inside a token is gone", ArithDoubleQuoteRemoved, `1"0"`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := parseArithOf(t, tc.src, quoted(tc.p))
			if tc.read != (got != nil) {
				t.Errorf("$((%s)) = %T, want read=%v", tc.src, got, tc.read)
			}
		})
	}
}

// Removed is the reading that joins the digits, and the tree says so: the
// bytes are gone before the expression is read, so `1"0"` is one literal and
// `n"a"me` is one name.
func TestARemovedDoubleQuoteJoinsTheToken(t *testing.T) {
	d := Core()
	d.ArithDoubleQuote = ArithDoubleQuoteRemoved
	if n, ok := parseArithOf(t, `1"0"`, d).(*ArithNum); !ok || n.Text != "10" {
		t.Errorf(`1"0" = %+v, want the literal 10`, n)
	}
	if v, ok := parseArithOf(t, `n"a"me`, d).(*ArithVar); !ok || v.Name != "name" {
		t.Errorf(`n"a"me = %+v, want the name "name"`, v)
	}
}

// Skipped reads through a quote standing where a token may begin, and the
// operand behind it is read as itself: a name is still a name.
func TestASkippedDoubleQuoteLeavesTheOperand(t *testing.T) {
	d := Core()
	d.ArithDoubleQuote = ArithDoubleQuoteSkipped
	if v, ok := parseArithOf(t, `"n"`, d).(*ArithVar); !ok || v.Name != "n" {
		t.Errorf(`"n" = %+v, want the name n`, v)
	}
	if n, ok := parseArithOf(t, `"1"`, d).(*ArithNum); !ok || n.Text != "1" {
		t.Errorf(`"1" = %+v, want the literal 1`, n)
	}
}

// The character constant is a literal one shell has and five refuse, so it is
// a flag. It builds the node the `##c` spelling builds, because it is the same
// question — the code of one character, escapes and all — and two nodes would
// be two places for the escape rules to drift.
func TestACharacterConstantNeedsTheFlag(t *testing.T) {
	d := Core()
	d.ArithCharacterConstant = true
	for _, tc := range []struct{ src, char string }{
		{`'a'`, "a"},
		{`'1'`, "1"},
		{`'\n'`, `\n`},
		{`'\101'`, `\101`},
		// The closing quote is optional, which is what makes `''` the code of
		// a quote: the second one is the character, with nothing left to
		// close it.
		{`''`, "'"},
	} {
		got, ok := parseArithOf(t, tc.src, d).(*ArithCharCode)
		if !ok || got.Op != "''" || got.Char != tc.char {
			t.Errorf("%s = %+v, want the character %q", tc.src, got, tc.char)
		}
	}
	// Without the flag the quote is no part of any token, which is the
	// missing operand five shells report.
	if got := parseArithOf(t, `'a'`, Core()); got != nil {
		t.Errorf("$(( 'a' )) = %T without ArithCharacterConstant, want nothing", got)
	}
	// Only one character is read, so `'ab'` leaves `b'` where an operator
	// belongs — which is the syntax error that shell gives rather than a
	// multi-character constant.
	if got := parseArithOf(t, `'ab'`, d); got != nil {
		t.Errorf("$(( 'ab' )) = %T, want the leftover refused", got)
	}
}
