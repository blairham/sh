// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// An underscore inside a numeral is a digit separator where the dialect says
// so: it is skipped, and the numeral reaches past it.
//
// Measured on zsh 5.9.2, 2026-09-13. `$(( 1_ ))` is the row the issue was
// filed on and it cannot decide anything — 1 is what a separator gives and
// also what a discarded byte gives — so every row here is one that parts the
// readings. What the parser answers is *how far the numeral reaches*; what it
// comes to is interp's, and the corpus rows under `arith/` grade the pair.
// #2223.
func TestAnUnderscoreInsideANumeralIsADigitSeparator(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{`1_0`, "1_0"},
		{`1_0_0`, "1_0_0"},
		{`1__0`, "1__0"},
		// A trailing one belongs to the numeral it follows: `1_ 2` is
		// `operator expected at `2'` on that shell, not at the underscore.
		{`1_`, "1_"},
		{`0_`, "0_"},
		// A based numeral is read the same way, and one may stand where the
		// first digit would.
		{`0x1_f`, "0x1_f"},
		{`0x_1`, "0x_1"},
		{`0b1_0`, "0b1_0"},
		{`2#1_0`, "2#1_0"},
		{`2#_10`, "2#_10"},
		{`1_0#5`, "1_0#5"},
		// And a float, in the fraction and in the exponent alike.
		{`1_0.5`, "1_0.5"},
		{`1.5_0`, "1.5_0"},
		{`1._5`, "1._5"},
		{`1e1_0`, "1e1_0"},
		{`1_e2`, "1_e2"},
		{`1e_2`, "1e_2"},
	} {
		if got, rest := separatedNumeral(t, c.expr, true); got != c.want || rest {
			t.Errorf("%q: numeral %q (leftover %v), want %q whole", c.expr, got, rest, c.want)
		}
	}
}

// The separator hides the byte it stands on and no other, so the numeral
// still ends where its base runs out.
//
// The pair for the table above, and it is what says this is a separator
// rather than "an underscore widens the alphabet": `1_abc` reaches past the
// underscore and stops at the `a`, which real zsh reports by blaming `abc`.
func TestASeparatorDoesNotHideTheByteAfterIt(t *testing.T) {
	for _, c := range []struct{ expr, blamed string }{
		{`1_abc`, "abc"},
		{`0x1_g`, "g"},
		{`2#1_2`, "2"},
		// An exponent needs a digit of its own, and a separator is not one:
		// `1e_` is `operator expected` there, the numeral having ended at the
		// `1`. We stop in the same place and name one byte more of the tail —
		// that shell reports the tail with its separators already gone, which
		// is a wording rather than a reading. See
		// syntax.Dialect.ArithDigitSeparator.
		{`1e_`, "e_"},
	} {
		if got, rest := separatedNumeral(t, c.expr, true); !rest || got != c.blamed {
			t.Errorf("%q: blamed %q (leftover %v), want %q left standing", c.expr, got, rest, c.blamed)
		}
	}
}

// And a dialect without the flag reads none of it.
//
// The same texts through the reader that stops at a bad digit and has no
// separator: the numeral ends at the underscore and the rest is left where an
// operator belongs. Without this the table above would pass for a parser that
// had always accepted the byte.
func TestWithoutTheFlagAnUnderscoreEndsTheNumeral(t *testing.T) {
	for _, c := range []struct{ expr, blamed string }{
		{`1_0`, "_0"},
		{`0x1_f`, "_f"},
		{`1_`, "_"},
	} {
		if got, rest := separatedNumeral(t, c.expr, false); !rest || got != c.blamed {
			t.Errorf("%q: blamed %q (leftover %v), want %q left standing", c.expr, got, rest, c.blamed)
		}
	}
}

// separatedNumeral parses one numeral through the reader that stops at a
// character its base cannot use, with the separator on or off.
func separatedNumeral(t *testing.T, expr string, sep bool) (text string, rest bool) {
	t.Helper()
	d := syntax.Core()
	d.ArithExplicitBase = true
	d.ArithNumeralEndsAtABadDigit = true
	d.ArithBinaryLiteral = true
	d.ArithFloat = true
	d.ArithDigitSeparator = sep
	p := syntax.NewParser("", d)
	e := p.ParseArithFor(expr, syntax.Pos{})
	if err := p.Err(); err != nil {
		var se *syntax.Error
		if errors.As(err, &se) {
			return se.Token, true
		}
		return "", true
	}
	n, ok := e.(*syntax.ArithNum)
	if !ok {
		t.Fatalf("%q: parsed to %T, want a numeral", expr, e)
	}
	return n.Text, false
}
