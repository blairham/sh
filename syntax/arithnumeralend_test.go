// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package syntax_test

import (
	"errors"
	"testing"

	"github.com/blairham/sh/syntax"
)

// Where a numeral ends, which is a question about how many tokens the text is
// and so belongs to the grammar rather than to a wording.
//
// One reader takes every character the base-64 alphabet knows and leaves the
// conversion to refuse what the base cannot use; the other stops at the first
// character its own base cannot use and leaves the rest standing, where an
// operator then belongs. `$(( 1abc ))` is one token to the first and two to
// the second (#2007).

func numeralText(t *testing.T, expr string, stops bool) (text string, rest bool) {
	t.Helper()
	d := syntax.Core()
	d.ArithExplicitBase = true
	d.ArithNumeralEndsAtABadDigit = stops
	d.ArithBinaryLiteral = stops
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

func TestANumeralTakesEveryCharacterTheAlphabetKnows(t *testing.T) {
	for _, c := range []struct{ expr, want string }{
		{`1abc`, "1abc"},
		{`0y`, "0y"},
		{`1@2`, "1@2"},
		{`1_`, "1_"},
		{`0x1f`, "0x1f"},
		{`8#9`, "8#9"},
	} {
		if got, rest := numeralText(t, c.expr, false); got != c.want || rest {
			t.Errorf("%q: numeral %q (leftover %v), want %q whole", c.expr, got, rest, c.want)
		}
	}
}

func TestANumeralStopsAtACharacterItsBaseCannotUse(t *testing.T) {
	for _, c := range []struct{ expr, blamed string }{
		// Decimal, so a letter ends it and is left where an operator belongs.
		{`1abc`, "abc"},
		{`0y`, "y"},
		// The base is known from the text, so the reader stops where that
		// base runs out rather than where the letters start.
		{`2#12`, "2"},
		{`08#9`, "9"},
		{`0b2`, "2"},
	} {
		if got, rest := numeralText(t, c.expr, true); !rest || got != c.blamed {
			t.Errorf("%q: blamed %q (leftover %v), want %q left over", c.expr, got, rest, c.blamed)
		}
	}
	// And a numeral its base *can* use is one token under either reader,
	// which is what says the flag moves where the scan stops and not what it
	// accepts.
	for _, c := range []struct{ expr, want string }{
		{`0x1f`, "0x1f"},
		{`0b101`, "0b101"},
		{`16#ff`, "16#ff"},
		{`123`, "123"},
	} {
		if got, rest := numeralText(t, c.expr, true); got != c.want || rest {
			t.Errorf("%q: numeral %q (leftover %v), want %q whole", c.expr, got, rest, c.want)
		}
	}
}
