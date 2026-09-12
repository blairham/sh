// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A radix prefix with nothing after it — `0x` — which two shells read as a
// complete number worth zero and two refuse.
//
// The value is asserted *and* the expression is asked to carry on past it,
// because a reader that answered zero by swallowing whatever followed the
// prefix would pass the first assertion alone. `0x+1` is 1 in the shells that
// take it, so the prefix finished and the addition happened.

func runRadix(t *testing.T, empty Answer, binary bool, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(d *syntax.Dialect) { d.ArithBinaryLiteral = binary },
		func(r *Runner) {
			sem := *r.Semantics
			sem.ArithEmptyRadixDigitsAreZero = empty
			r.Semantics = &sem
		})
}

func TestARadixPrefixWithNoDigitsIsZeroWhereTheDialectSaysSo(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "v=$(( 0x ))"`, "v=0"},
		{`echo "v=$(( 0X ))"`, "v=0"},
		{`echo "v=$(( 0x+1 ))"`, "v=1"},
		{`echo "v=$(( 1+0x ))"`, "v=1"},
	} {
		if out, st := runRadix(t, Yes, false, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("zero, %s: got %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
		if out, st := runRadix(t, No, false, c.src); strings.Contains(out, "v=") || st == 0 {
			t.Errorf("refused, %s: got %q (status %d), want no value and a failure", c.src, out, st)
		}
	}
	// A prefix with a digit after it is untouched by the answer, which is
	// what says the axis is about the empty run and not about the prefix.
	for _, empty := range []Answer{Yes, No} {
		if out, st := runRadix(t, empty, false, `echo "v=$(( 0x1f ))"`); strings.TrimSpace(out) != "v=31" || st != 0 {
			t.Errorf("empty=%v: got %q (status %d), want v=31 at 0", empty, out, st)
		}
	}
}

// The binary prefix is the grammar flag rather than the axis, and the two meet
// on the empty run: where the dialect has `0b` at all, `0b` with nothing after
// it answers the same way `0x` does.
func TestABinaryRadixPrefixWhereTheDialectHasOne(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo "v=$(( 0b101 ))"`, "v=5"},
		{`echo "v=$(( 0B101 ))"`, "v=5"},
		{`echo "v=$(( 0b101+1 ))"`, "v=6"},
		{`echo "v=$(( 0b ))"`, "v=0"},
		{`echo "v=$(( 0b+1 ))"`, "v=1"},
	} {
		if out, st := runRadix(t, Yes, true, c.src); strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
	// Without the flag the same text is a leading-zero numeral carrying a
	// `b`, which is what the shells without the prefix make of it — not a
	// binary literal read a different way.
	for _, src := range []string{`echo "v=$(( 0b101 ))"`, `echo "v=$(( 0b ))"`} {
		if out, st := runRadix(t, Yes, false, src); strings.Contains(out, "v=") || st == 0 {
			t.Errorf("no flag, %s: got %q (status %d), want no value and a failure", src, out, st)
		}
	}
}
