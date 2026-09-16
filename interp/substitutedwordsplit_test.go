// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The word a `-` or `+` substitutes is part of an unquoted expansion's result,
// so what the script wrote in it separates fields: `${v:+p q}` is two fields
// where an unquoted expansion splits and one where it does not.
//
// Tests here name the axis and never a shell. The field *count* is asserted
// beside the text, because a word that lost a boundary prints back as the same
// characters and nothing shorter than the count can see the difference.

// substRun runs src with the splitting axis answered as given.
func substRun(t *testing.T, src string, split Answer) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.SplitParamExpansion = split
		s.EmptyQuotesAfterASeparatorAreAField = Yes
	})
}

const substPrinter = `pr() { printf "[%s]" "$@"; printf "\n"; }; set -- a b; v=x; `

func TestTheSubstitutedWordSplits(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, src, split, unsplit string
	}{
		{
			name:    "blanks the word was written with",
			src:     `pr ${v:+p q}`,
			split:   "[p][q]",
			unsplit: "[p q]",
		},
		{
			name:    "the same word an alternative did not reach",
			src:     `pr ${nope:-p q}`,
			split:   "[p][q]",
			unsplit: "[p q]",
		},
		{
			name:    "a blank between two lists, which keep their own boundaries",
			src:     `pr ${v:+"$@" "$@"}`,
			split:   "[a][b][a][b]",
			unsplit: "[a][b a][b]",
		},
		{
			name:    "quoted text in the word is not split",
			src:     `pr ${v:+"p q"}`,
			split:   "[p q]",
			unsplit: "[p q]",
		},
		{
			name:    "a separator IFS does not name is text",
			src:     `IFS=:; pr ${v:+p q}`,
			split:   "[p q]",
			unsplit: "[p q]",
		},
		{
			name:    "and one it does names a boundary",
			src:     `IFS=:; pr ${v:+p:q}`,
			split:   "[p][q]",
			unsplit: "[p:q]",
		},
		{
			name:    "a blank ending the word opens no field",
			src:     `pr ${v:+p }`,
			split:   "[p]",
			unsplit: "[p ]",
		},
		{
			name:    "and one beginning it opens none either",
			src:     `pr ${v:+ p}`,
			split:   "[p]",
			unsplit: "[ p]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, c.split}, {No, c.unsplit}} {
				out, _ := substRun(t, substPrinter+c.src, a.answer)
				if got := trimLine(out); got != a.want {
					t.Errorf("split=%v: %q, want %q", a.answer, got, a.want)
				}
			}
		})
	}
}

// The contexts that keep one word do not reach the question at all, which is
// unanimous: an assignment's right-hand side, a `case` subject and a `[[ ]]`
// operand keep the word whole whatever the axis says.
func TestTheSubstitutedWordIsNotSplitWhereNothingKeepsFields(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"an assignment's right-hand side", `x=${v:+p q}; pr "$x"`, "[p q]"},
		{"a case subject", `case ${v:+p q} in "p q") pr yes;; *) pr no;; esac`, "[yes]"},
		{"the word quoted whole", `pr "${v:+p q}"`, "[p q]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, a := range []Answer{Yes, No} {
				out, _ := substRun(t, substPrinter+c.src, a)
				if got := trimLine(out); got != c.want {
					t.Errorf("split=%v: %q, want %q", a, got, c.want)
				}
			}
		})
	}
}

// A quoted empty word written behind a separator in the substituted word is a
// field of its own in four of the five columns and is not one in the fifth, so
// it is an axis rather than a consequence of the split.
func TestEmptyQuotesBehindASeparatorAreAnAxis(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, isField, isNot string }{
		{"after a word", `pr ${v:+p ""}`, "[p][]", "[p]"},
		{"after a list", `pr ${v:+"$@" ""}`, "[a][b][]", "[a][b]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, a := range []struct {
				answer Answer
				want   string
			}{{Yes, c.isField}, {No, c.isNot}} {
				out, _ := axisRun(t, substPrinter+c.src, func(s *Semantics) {
					s.SplitParamExpansion = Yes
					s.EmptyQuotesAfterASeparatorAreAField = a.answer
				})
				if got := trimLine(out); got != a.want {
					t.Errorf("field=%v: %q, want %q", a.answer, got, a.want)
				}
			}
		})
	}
}

// A separator at either end of what an unquoted expansion came to closes the
// field beside it, even where the separators were the whole of the value and
// no field came out of the split at all. Unanimous — this is a correction
// rather than an axis — and it reaches an expansion, a command substitution
// and a value with a separator at one end alike.
func TestASeparatorClosesTheFieldEvenWithNothingBesideIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a value that is nothing but a separator", `v=" "; pr a${v}b`, "[a][b]"},
		{"a separator the value begins with", `v=" b"; pr a$v`, "[a][b]"},
		{"a separator the value ends with", `v="b "; pr ${v}c`, "[b][c]"},
		{"a command substitution that printed blanks", `pr a$(printf " ")b`, "[a][b]"},
		{"an empty value, which closes nothing", `v=""; pr a${v}b`, "[ab]"},
		{"a value of separators alone, which opens no field", `v=" "; pr $v`, "[]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := substRun(t, `pr() { printf "[%s]" "$@"; printf "\n"; }; `+c.src, Yes)
			if got := trimLine(out); got != c.want {
				t.Errorf("%q, want %q", got, c.want)
			}
		})
	}
}

// trimLine is the one line these cases print, without its newline.
func trimLine(out string) string { return strings.TrimSuffix(out, "\n") }
