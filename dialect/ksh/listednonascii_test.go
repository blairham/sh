// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// A non-ASCII **character** is written as itself in every listing surface
// here, and a high byte that is not one is spelled out — #4770.
//
// Measured 2026-09-27 on `/bin/ksh`, `Version AJM 93u+ 2012-08-01`, from a
// script file under `env -i PATH=/usr/bin:/bin`; `go version -m` says *not a
// Go executable* for it and `github.com/blairham/sh/cmd/ksh` for ours.
//
// **The locale is the input that decides it**, which is why the axis read the
// other way for as long as it did. One binary, the locale the only thing that
// moved:
//
//	                         LC_ALL=C              LC_ALL=en_US.UTF-8
//	u=é; typeset -p u        u=$'\xc3\xa9'         u=é
//	w=$'\xc3\xa9'            w=$'\xc3\xa9'         w=é
//	alias al=é; alias al     al=$'\xc3\xa9'        al=é
//	m[é]=é; typeset -p m     [$'\xc3\xa9']=$'…'    [é]=é
//	v=$'\xc3'; typeset -p v  v=$'\xc3'             v=$'\xc3'
//
// The recorded value came from the first column — the corpus harness pins
// `LC_ALL=C` — and its stated reason was that this shell spells the byte out
// `in every locale`, which the second column contradicts. bash answers the
// same axis the same way round, and `dialect/bash` already states the choice
// this now follows: the preset carries the reading a person's terminal sees,
// and the corpus goes on recording the `C` cell.
//
// The last row is the control and it is what makes this `Yes` rather than a
// third answer: a high byte that is **not** part of a character is spelled
// out in both locales, which is the distinction #4521 drew for the two
// columns that already answered `Yes`.
//
// **What a `$'...'` spells such a character as is a second question and is
// not here**: a word that needs quoting *and* holds one is `$'a \u[e9] b'`
// in a UTF-8 locale — a code-point form nothing in this engine writes — and
// so is `$'\xc3\xa9\xff'`, where the stray byte is what forces the form.
// Both are #4807. The axis decides whether the byte may stand bare, and those
// rows are about the escape.
func TestAListedNonAsciiCharacterIsWrittenAsItself(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `u=é; typeset -p u`, "u=é\n"},
		{"the same value written as bytes", `w=$'\xc3\xa9'; typeset -p w`, "w=é\n"},
		{"an alias body", `alias al=é; alias al`, "al=é\n"},
		{
			"a table's key and its value",
			`typeset -A m; m[é]=é; typeset -p m`,
			"typeset -A m=([é]=é)\n",
		},
		// The control, in three shapes: a byte that cannot be a character on
		// its own is escaped wherever it stands, and a value holding both
		// writes the character and escapes the byte.
		{"a lone lead byte", `v=$'\xc3'; typeset -p v`, "v=$'\\xc3'\n"},
		// `\xc1z` and not `\xc1b`: this shell's `\x` reads as many hex digits
		// as it finds, so `\xc1b` is one three-digit code point and the row
		// would be about the escape rather than about the byte.
		{"one in the middle of a value", `y=$'a\xc1z'; typeset -p y`, "y=$'a\\xc1z'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The second question the row above named and left: what a `$'...'` spells
// such a character as, and what takes a value into the form in the first
// place — #4807.
//
// Measured 2026-09-27 on `/bin/ksh`, `Version AJM 93u+ 2012-08-01`, from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=en_US.UTF-8`; `go
// version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/ksh` for ours.
//
// Two facts, and only the second is about the spelling. **The form is reached
// where an ASCII character a name cannot hold stands in front of the
// character** — `a-é` and `a.é` take it where `aé`, `a9é` and `é-a` are bare
// — and **inside it the character is a code point**, `\u[e9]`, which this
// engine had no form for at all.
//
// The rows are the UTF-8 locale's and this engine is locale-blind, which is
// the choice #4770 made for the axis beside this one: the preset carries the
// reading a person's terminal sees and the corpus goes on recording the
// `LC_ALL=C` cell. So these assert the same bytes whatever locale the test
// runs in, which is the claim the preset is making.
//
// **Two things measured in the same run are deliberately not here.** That
// column also refuses to leave some characters above ASCII bare at all —
// `€`, a non-breaking space, `°`, `²`, `½`, `×`, a soft hyphen, a combining
// acute and an emoji each take the form on their own, where `é`, `µ`, `中`,
// `å`, `٣`, `ʰ`, `Ⅷ` and `Ⓐ` do not — and it treats a character above ASCII
// as a name character in the `=` and `#` rules, so `é=a` is bare there and
// `é#a` is quoted, both the other way round here. Each is a rule of its own
// and neither is inferable from these rows; see nonAsciiAfterANonName.
func TestAListedNonAsciiCharacterIsACodePointInsideDollarQuotes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a value that needs quoting", `x="a é b"; typeset -p x`, "x=$'a \\u[e9] b'\n"},
		{"a character beside a stray byte", `z=$'\xc3\xa9\xff'; typeset -p z`, "z=$'\\u[e9]\\xff'\n"},
		{"a value holding a quote", `g="a'é"; typeset -p g`, "g=$'a\\'\\u[e9]'\n"},
		{"a value holding a control character", `f=$'a\téb'; typeset -p f`, "f=$'a\\t\\u[e9]b'\n"},
		// The two rows the issue could not account for, and they are the same
		// rule as the first: a `-` or a `.` is an ASCII character a name
		// cannot hold, so a character standing after one reaches the form.
		{"a hyphen in front of it", `v='a-é'; typeset -p v`, "v=$'a-\\u[e9]'\n"},
		{"a dot in front of it", `v='a.é'; typeset -p v`, "v=$'a.\\u[e9]'\n"},
		// Every such character is judged and not only the first: the leading
		// one here would stand bare on its own and is spelled out anyway.
		{"one of two failing", `v='é-é'; typeset -p v`, "v=$'\\u[e9]-\\u[e9]'\n"},
		// An assignment head does not move the decision: the tail on its own
		// would have stood bare.
		{"a bare assignment head", `v='a=é'; typeset -p v`, "v=a=$'\\u[e9]'\n"},
		// The form is not reached by a value merely needing quotes, which is
		// the control that keeps this about what stands *before* the
		// character.
		{"quoted for a blank, with nothing in front", `v='é b'; typeset -p v`, "v='é b'\n"},
		// And the bare rows, which have to keep standing bare.
		{"a name in front of it", `v='aé'; typeset -p v`, "v=aé\n"},
		{"a digit in a name in front of it", `v='a9é'; typeset -p v`, "v=a9é\n"},
		{"an underscore in front of it", `v='a_é'; typeset -p v`, "v=a_é\n"},
		{"nothing in front of it", `v='é-a'; typeset -p v`, "v=é-a\n"},
		{"a character above ASCII in front of it", `v='é9é'; typeset -p v`, "v=é9é\n"},
		{"a leading ASCII digit, which is no name", `v='9é'; typeset -p v`, "v=$'9\\u[e9]'\n"},
		// The code point's own shape, across the ranges: lower-case
		// hexadecimal with no leading zeros.
		{"a two-digit code point", `v='a-é'; typeset -p v`, "v=$'a-\\u[e9]'\n"},
		{"a three-digit code point", `v=$'a-\u0101'; typeset -p v`, "v=$'a-\\u[101]'\n"},
		{"a four-digit code point", `v=$'a-\u20ac'; typeset -p v`, "v=$'a-\\u[20ac]'\n"},
		{"a five-digit code point", `v=$'a-\U0001f600'; typeset -p v`, "v=$'a-\\u[1f600]'\n"},
		{"the top of the range", `v=$'a-\U0010ffff'; typeset -p v`, "v=$'a-\\u[10ffff]'\n"},
		// The two characters this column writes byte by byte instead, which
		// are measured rather than a class: the noncharacters either side of
		// them take the code point.
		{"U+FFFF", `v=$'a-\uffff'; typeset -p v`, "v=$'a-\\xef\\xbf\\xbf'\n"},
		{"U+FFFE", `v=$'a-\ufffe'; typeset -p v`, "v=$'a-\\xef\\xbf\\xbe'\n"},
		{"U+FDD0, which is not one of them", `v=$'a-\ufdd0'; typeset -p v`, "v=$'a-\\u[fdd0]'\n"},
		{"U+1FFFE, nor is this", `v=$'a-\U0001fffe'; typeset -p v`, "v=$'a-\\u[1fffe]'\n"},
		// And the same rule reaching the other listing surfaces, which is
		// what says it is the engine's and not `typeset -p`'s.
		{"an alias body", `alias al='a-é'; alias al`, "al=$'a-\\u[e9]'\n"},
		{
			"a table's key and its value",
			`typeset -A m; m['a-é']='a-é'; typeset -p m`,
			"typeset -A m=([$'a-\\u[e9]']=$'a-\\u[e9]')\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The axes those rows rest on, read off the vector — and the pair is what
// says the two questions are separate: a column could reach the form and
// still write the character as itself, which is what bash and zsh do for a
// value a control byte put them in.
func TestTheTwoNonAsciiListingAxesAreAnsweredHere(t *testing.T) {
	s := ksh.Semantics()
	if got := s.ListedNonAsciiIsSpelledAsACodePoint; got != interp.Yes {
		t.Errorf("ListedNonAsciiIsSpelledAsACodePoint = %v, want Yes", got)
	}
	if got := s.ListedNonAsciiTakesTheDollarFormAfterANonName; got != interp.Yes {
		t.Errorf("ListedNonAsciiTakesTheDollarFormAfterANonName = %v, want Yes", got)
	}
}
