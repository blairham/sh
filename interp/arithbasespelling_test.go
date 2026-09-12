// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// How a `base#digits` literal may be spelled, which is three questions in one
// reader and answered by three different sets of shells.
//
// The base itself is read in plain decimal everywhere it is read at all. What
// the axes decide is which texts are a base *spelling* — and a text that is
// not one is not an error either: it falls through to the ordinary numeral
// reading, where the `#` is simply a byte no base can use. That fall-through
// is bash's whole rule rather than a fallback, which is why the complaint it
// produces is asserted here beside the value.

// runBase runs src with the explicit-base grammar on and the two spelling
// axes answered, since a base literal turns on both.
func runBase(t *testing.T, leadingZero, twoDigits Answer, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src,
		func(d *syntax.Dialect) { d.ArithExplicitBase = true },
		func(r *Runner) {
			sem := *r.Semantics
			sem.ArithBaseMayHaveALeadingZero = leadingZero
			sem.ArithBaseIsAtMostTwoDigits = twoDigits
			sem.ArithLeadingZeroIsOctal = No
			r.Semantics = &sem
		})
}

// ArithBaseMayHaveALeadingZero, asked with a digit that tells the two possible
// readings of the base apart.
//
// `010#5` is five whether the base is eight or ten, which is the probe #2006
// was filed from and the reason it cannot stand alone: only `010#9` and
// `010#11` say the base was read in decimal, since nine is no octal digit and
// eleven is two of them.
func TestABaseMayBeWrittenWithALeadingZero(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`echo $((010#5))`, "5"},
		{`echo $((010#9))`, "9"},
		{`echo $((010#11))`, "11"},
		{`echo $((0010#5))`, "5"},
	} {
		out, st := runBase(t, Yes, No, c.src)
		if strings.TrimSpace(out) != c.want || st != 0 {
			t.Errorf("%s: got %q (status %d), want %q at 0", c.src, out, st, c.want)
		}
	}
}

// And under the other answer none of them is a base at all: the text is read
// as an ordinary numeral, which the `#` in it makes unreadable.
func TestABaseWrittenWithALeadingZeroIsNoBase(t *testing.T) {
	for _, src := range []string{`echo "v=$((010#5))"`, `echo "v=$((010#9))"`, `echo "v=$((0010#5))"`} {
		out, st := runBase(t, No, No, src)
		if strings.Contains(out, "v=") || st == 0 {
			t.Errorf("%s: got %q (status %d), want no value and a failure", src, out, st)
		}
	}
	// A base with no zero in front of it is untouched by the answer, which is
	// what says the axis is about the zero and not about bases.
	if out, st := runBase(t, No, No, `echo $((10#5))`); strings.TrimSpace(out) != "5" || st != 0 {
		t.Errorf("an unpadded base: got %q (status %d), want 5 at 0", out, st)
	}
}

// ArithBaseIsAtMostTwoDigits, which is a length rule and not a padding one —
// so it is asserted with the leading zero *allowed*, where the two axes would
// otherwise be indistinguishable.
func TestABaseStopsAfterTwoCharactersWhereTheDialectSaysSo(t *testing.T) {
	for _, c := range []struct {
		src         string
		capped, not string
	}{
		// Two characters, so the cap changes nothing.
		{`echo "v=$((02#11))"`, "3", "3"},
		{`echo "v=$((08#7))"`, "7", "7"},
		// Two characters is as many as the widest base needs.
		{`echo "v=$((64#10))"`, "64", "64"},
		// Four, so the cap takes `00` — which is no base — and refuses.
		{`echo "v=$((0002#11))"`, "", "3"},
		// Three, and the cap takes `01`, which is no base either.
		{`echo "v=$((010#5))"`, "", "5"},
	} {
		out, st := runBase(t, Yes, Yes, c.src)
		if c.capped == "" {
			if strings.Contains(out, "v=") || st == 0 {
				t.Errorf("capped, %s: got %q (status %d), want no value and a failure", c.src, out, st)
			}
		} else if strings.TrimSpace(out) != "v="+c.capped || st != 0 {
			t.Errorf("capped, %s: got %q (status %d), want %q at 0", c.src, out, st, c.capped)
		}
		if out, st := runBase(t, Yes, No, c.src); strings.TrimSpace(out) != "v="+c.not || st != 0 {
			t.Errorf("uncapped, %s: got %q (status %d), want %q at 0", c.src, out, st, c.not)
		}
	}
}

// A base outside two through sixty-four is refused rather than fallen through
// from, which is the line between this and the two axes above: `1#0` names a
// base and cannot have one, where `010#5` names no base at all.
func TestABaseALiteralNamesOutsideTheAlphabetIsRefused(t *testing.T) {
	for _, src := range []string{`echo "v=$((1#0))"`, `echo "v=$((0#5))"`, `echo "v=$((65#1))"`} {
		out, st := runBase(t, Yes, No, src)
		if strings.Contains(out, "v=") || st == 0 {
			t.Errorf("%s: got %q (status %d), want no value and a failure", src, out, st)
		}
	}
}

// The two complaints a literal can earn, parted by the base-64 alphabet rather
// than by the base in hand: a byte the alphabet knows is a digit this base
// cannot reach, and a byte it does not know is no digit anywhere. One dialect
// words them differently and the rest word them alike, so the wording is read
// through Diagnostics and the *choice* is made here.
func TestALiteralIsBlamedOnWhetherTheByteIsADigitAtAll(t *testing.T) {
	const tooGreat, noDigit = "too great", "no digit"
	for _, c := range []struct{ src, want string }{
		// `8` is a digit and base eight has none.
		{`echo $((08))`, tooGreat},
		{`echo $((2#12))`, tooGreat},
		// `#` is no digit in any base, the leading zero having made the text
		// an octal constant before anything looked for a base.
		{`echo $((010#5))`, noDigit},
		{`echo $((0#5))`, noDigit},
	} {
		out, st := runGrammar(t, c.src,
			func(d *syntax.Dialect) { d.ArithExplicitBase = true },
			func(r *Runner) {
				sem := *r.Semantics
				sem.ArithBaseMayHaveALeadingZero = No
				sem.ArithBaseIsAtMostTwoDigits = No
				sem.ArithLeadingZeroIsOctal = Yes
				sem.ArithInvalidOctalDigitIsError = Yes
				r.Semantics = &sem
				r.Diagnostics = &Diagnostics{
					DigitTooGreatForBase: tooGreat,
					ArithByteIsNoDigit:   noDigit,
				}
			})
		if !strings.Contains(out, c.want) || st == 0 {
			t.Errorf("%s: got %q (status %d), want %q", c.src, out, st, c.want)
		}
	}
}

// And with no separate wording the two collapse onto one sentence, which is
// what ksh93 and dash do — asserted so that an empty field is a measured
// answer rather than a gap the default happens to fill.
func TestOneSentenceCoversBothWhereTheDialectHasNoSecond(t *testing.T) {
	const only = "one sentence"
	for _, src := range []string{`echo $((08))`, `echo $((010#5))`} {
		out, _ := runGrammar(t, src,
			func(d *syntax.Dialect) { d.ArithExplicitBase = true },
			func(r *Runner) {
				sem := *r.Semantics
				sem.ArithBaseMayHaveALeadingZero = No
				sem.ArithLeadingZeroIsOctal = Yes
				sem.ArithInvalidOctalDigitIsError = Yes
				r.Semantics = &sem
				r.Diagnostics = &Diagnostics{DigitTooGreatForBase: only}
			})
		if !strings.Contains(out, only) {
			t.Errorf("%s: got %q, want %q", src, out, only)
		}
	}
}
