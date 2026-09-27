// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Which characters above ASCII a listing may leave bare, and what counts as a
// **name** in the two rules that read a value's leading text — see
// Semantics.ListedNonAsciiIsBareOnlyWhenAlphabetic, listedNameLike, #4829 and
// #4830.
//
// Tests name axes and never shells.

// listedCharRun lists a scalar under a style that reaches for `$'...'` and
// spells a character above ASCII inside it as a code point, which is the only
// column either rule is live in.
func listedCharRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	return declareRun(t, src, func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.DeclareValueQuoting = ListingQuoteWhenNeededDollar
		s.ListedNonAsciiIsOrdinary = Yes
		s.ListedNonAsciiIsSpelledAsACodePoint = Yes
		s.ListedNonAsciiTakesTheDollarFormAfterANonName = Yes
		s.ListedAssignmentPrefixIsBare = Yes
		s.ListedHashIsBareAfterANonName = Yes
		if set != nil {
			set(s)
		}
	}, Diagnostics{})
}

// Both answers over one set of rows, because a suite that ran only the new
// one could not tell a fix from a hardcoding.
//
// The bare column is what every dialect but one does with every one of these
// characters; the narrowed column leaves only the alphabetic ones alone.
func TestANonAlphabeticCharacterAboveAsciiIsSettledBothWays(t *testing.T) {
	for _, tc := range []struct {
		narrowed        Answer
		degree, euro    string
		times, emoji    string
		nbsp, combining string
	}{
		{
			No,
			"v=°", "v=€", "v=×", "v=😀", "v= ", "v=́",
		},
		{
			Yes,
			`v=$'\u[b0]'`, `v=$'\u[20ac]'`, `v=$'\u[d7]'`,
			`v=$'\u[1f600]'`, `v=$'\u[a0]'`, `v=$'\u[301]'`,
		},
	} {
		t.Run(tc.narrowed.String(), func(t *testing.T) {
			for _, row := range []struct{ src, want string }{
				{`v='°'; typeset -p v`, tc.degree},
				{`v='€'; typeset -p v`, tc.euro},
				{`v='×'; typeset -p v`, tc.times},
				{"v=$'\U0001f600'; typeset -p v", tc.emoji},
				{"v=$' '; typeset -p v", tc.nbsp},
				{"v=$'́'; typeset -p v", tc.combining},
			} {
				out, errs, st := listedCharRun(t, row.src, func(s *Semantics) {
					s.ListedNonAsciiIsBareOnlyWhenAlphabetic = tc.narrowed
				})
				if strings.TrimSuffix(out, "\n") != row.want || errs != "" || st != 0 {
					t.Errorf("%s = %q (stderr %q, status %d), want %q",
						row.src, out, errs, st, row.want)
				}
			}
		})
	}
}

// The characters the narrowed answer still leaves bare, and three of them are
// what says the reading is an alphabetic **class** rather than a Unicode
// general category: a decimal digit, a letter-number and an enclosed letter
// are each bare, where two other symbols in the suite above are not.
func TestAnAlphabeticCharacterAboveAsciiStaysBare(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a letter", `v='é'; typeset -p v`, "v=é"},
		{"a micro sign", `v='µ'; typeset -p v`, "v=µ"},
		{"a CJK ideograph", `v='中'; typeset -p v`, "v=中"},
		{"a modifier letter", `v='ʰ'; typeset -p v`, "v=ʰ"},
		{"a decimal digit", `v='٣'; typeset -p v`, "v=٣"},
		{"a letter-number", `v='Ⅷ'; typeset -p v`, "v=Ⅷ"},
		{"an enclosed letter", `v='Ⓐ'; typeset -p v`, "v=Ⓐ"},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := listedCharRun(t, row.src, func(s *Semantics) {
				s.ListedNonAsciiIsBareOnlyWhenAlphabetic = Yes
			})
			if strings.TrimSuffix(out, "\n") != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q",
					row.src, out, errs, st, row.want)
			}
		})
	}
}

// One character failing takes the whole value into the form, which is the
// half a per-character fix would get wrong: the `é` beside it would have
// stood bare on its own and is spelled out anyway.
func TestOneNonAlphabeticCharacterTakesTheWholeValue(t *testing.T) {
	for _, row := range []struct{ src, want string }{
		{`v='é°'; typeset -p v`, `v=$'\u[e9]\u[b0]'`},
		{`v='°é'; typeset -p v`, `v=$'\u[b0]\u[e9]'`},
		{`v='x°y'; typeset -p v`, `v=$'x\u[b0]y'`},
		// A value that would otherwise have been quoted for its blank is
		// taken into the form instead, and the control beside it keeps its
		// quotes — which is the pair that says this is the character's doing.
		{`v='° b'; typeset -p v`, `v=$'\u[b0] b'`},
		{`v='é b'; typeset -p v`, `v='é b'`},
	} {
		out, errs, st := listedCharRun(t, row.src, func(s *Semantics) {
			s.ListedNonAsciiIsBareOnlyWhenAlphabetic = Yes
		})
		if strings.TrimSuffix(out, "\n") != row.want || errs != "" || st != 0 {
			t.Errorf("%s = %q (stderr %q, status %d), want %q",
				row.src, out, errs, st, row.want)
		}
	}
}

// A character above ASCII is a **name** character in the `=` and `#` rules,
// and the two go opposite ways over it — which is what says they are one fact
// rather than two. A leading `name=` is written bare, so a name made of such
// a character leaves the head bare; a `#` is left bare only where the text in
// front of it is *not* a name, so the same character quotes it.
func TestACharacterAboveAsciiCountsAsAName(t *testing.T) {
	for _, row := range []struct{ name, src, want string }{
		{"a bare assignment head", `v='é=a'; typeset -p v`, "v=é=a"},
		{"and the hash rule the other way", `v='é#a'; typeset -p v`, `v='é#a'`},
		{"a digit behind one is still a name", `v='é9é=a'; typeset -p v`, "v=é9é=a"},
		{"and its hash row", `v='é9é#a'; typeset -p v`, `v='é9é#a'`},
		{"a decimal digit above ASCII, at the front", `v='٣=a'; typeset -p v`, "v=٣=a"},
		{"an enclosed letter", `v='Ⓐ=a'; typeset -p v`, "v=Ⓐ=a"},
		{"and its hash row", `v='Ⓐ#a'; typeset -p v`, `v='Ⓐ#a'`},
		// The ASCII controls, which agreed before any of this.
		{"an ASCII name and an equals", `v='a=b'; typeset -p v`, "v=a=b"},
		{"no name in front of the equals", `v='=x'; typeset -p v`, `v='=x'`},
		{"a leading digit is no name", `v='1=2'; typeset -p v`, `v='1=2'`},
		{"an ASCII name and a hash", `v='a#b'; typeset -p v`, `v='a#b'`},
		{"no name in front of the hash", `v='1#b'; typeset -p v`, "v=1#b"},
		{"what looks like a base", `v='16#ff'; typeset -p v`, "v=16#ff"},
		// And a character the listing will not leave bare is no name either,
		// so the head does not split and the whole value takes the form.
		{"a non-alphabetic one is no name", `v='°=a'; typeset -p v`, `v=$'\u[b0]=a'`},
		// The head really is bare, which this says by keeping the tail's own
		// answer: the tail reaches the form and the head does not move.
		{"a bare head and a dollar tail", `v='a=é'; typeset -p v`, `v=a=$'\u[e9]'`},
		{"the same with a name above ASCII", `v='é=°'; typeset -p v`, `v=é=$'\u[b0]'`},
	} {
		t.Run(row.name, func(t *testing.T) {
			out, errs, st := listedCharRun(t, row.src, func(s *Semantics) {
				s.ListedNonAsciiIsBareOnlyWhenAlphabetic = Yes
			})
			if strings.TrimSuffix(out, "\n") != row.want || errs != "" || st != 0 {
				t.Errorf("%s = %q (stderr %q, status %d), want %q",
					row.src, out, errs, st, row.want)
			}
		})
	}
}
