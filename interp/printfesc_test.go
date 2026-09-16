// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The escape character in a printf *format*, at both of its spellings (#3225).
//
// `printf '\e[1m'` is the colour idiom, and the format's escape reader had
// neither letter before this: six of the seven panel columns write an ESC for
// `\e` and four write one for `\E`, and this shell wrote the backslash and
// the letter in every dialect.
//
// Both axes are moved in both directions and left unanswered, because an
// escape that is only ever taken cannot be told from one that is always
// taken: the declining rows are what say the axis is read at all.
func TestPrintfFormatEscapeAxes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  func(*Semantics)
		src     string
		want    string
		refused string
	}{
		{
			name: "esc, taken", src: `printf 'a\eZ'`, want: "a\x1bZ",
			answer: func(s *Semantics) { s.PrintfEscEscape = Yes },
		},
		{
			name: "esc, declined", src: `printf 'a\eZ'`, want: `a\eZ`,
			answer: func(s *Semantics) { s.PrintfEscEscape = No },
		},
		{
			name: "esc, unanswered", src: `printf 'a\eZ'`, want: `a\eZ`,
			answer:  func(s *Semantics) { s.PrintfEscEscape = Unspecified },
			refused: `printf: \e in a format`,
		},
		{
			name: "capital esc, taken", src: `printf 'a\EZ'`, want: "a\x1bZ",
			answer: func(s *Semantics) { s.PrintfCapitalEscEscape = Yes },
		},
		{
			name: "capital esc, declined", src: `printf 'a\EZ'`, want: `a\EZ`,
			answer: func(s *Semantics) { s.PrintfCapitalEscEscape = No },
		},
		{
			name: "capital esc, unanswered", src: `printf 'a\EZ'`, want: `a\EZ`,
			answer:  func(s *Semantics) { s.PrintfCapitalEscEscape = Unspecified },
			refused: `printf: \E in a format`,
		},
		{
			// The idiom itself, and the reason this is a P1 rather than a
			// completeness item: a format is reused over its operands and
			// the escape is read afresh each pass.
			name: "the colour idiom", src: `printf '\e[1m%s\e[0m' bold`, want: "\x1b[1mbold\x1b[0m",
			answer: func(s *Semantics) { s.PrintfEscEscape = Yes },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			tc.answer(&sem)
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused != "" {
				// The same shape `\x` takes at this site: the refusal names
				// the axis, the escape still stands, and the status is 2.
				want := "sh: " + tc.refused + ": the shells disagree here and no dialect was chosen\n" + tc.want
				if out != want || st != 2 {
					t.Errorf("got %q status %d, want %q and 2", out, st, want)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The two letters are two axes at this site as they are at the `%b` site,
// and the four combinations are all reachable: dash has neither, zsh and
// BusyBox ash have the small one alone, the bash builds and ksh93 have both.
// The fourth is nothing in the panel and is asserted all the same, because a
// pair of fields that could not express it would be one field.
func TestPrintfFormatEscAndCapitalEscAreTwoQuestions(t *testing.T) {
	for _, tc := range []struct {
		name       string
		esc, capes Answer
		want       string
	}{
		{"bash and ksh93 have both", Yes, Yes, "a\x1bZ:a\x1bZ"},
		{"dash has neither", No, No, `a\eZ:a\EZ`},
		{"zsh and ash have the small alone", Yes, No, "a\x1bZ:a\\EZ"},
		{"the capital alone", No, Yes, "a\\eZ:a\x1bZ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfEscEscape = tc.esc
			sem.PrintfCapitalEscEscape = tc.capes
			out, st := run(t, `printf 'a\eZ:a\EZ'`, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}

// The format's pair and the `%b` argument's pair are four fields and not two,
// which is what ksh93u+ needs: it takes `\e` in a format and writes the two
// characters in a `%b`. Both sites are asked in one run and set against each
// other, so a reader that fell back on the other site's axis would show here
// and nowhere else.
func TestTheFormatAndTheBArgumentAskTheirOwnEscapeAxes(t *testing.T) {
	sem := printfSem()
	sem.PrintfEscEscape = Yes
	sem.PrintfCapitalEscEscape = Yes
	sem.PrintfBEscEscape = No
	sem.PrintfBCapitalEscEscape = Yes
	out, st := run(t, `printf 'a\eZ|a\EZ|%b|%b' 'a\eZ' 'a\EZ'`,
		func(r *Runner) { r.Semantics = &sem })
	want := "a\x1bZ|a\x1bZ|a\\eZ|a\x1bZ"
	if out != want || st != 0 {
		t.Errorf("got %q status %d, want %q and 0", out, st, want)
	}
}

// An axis is asked only where its escape is in the format, so a format that
// carries neither letter needs no dialect at all — the same rule the `%b`
// site's pair follows, and the reason a script with no colours in it is not
// refused by a shell with no dialect.
func TestPrintfFormatEscapeAxesAreAskedOnlyWhenTheEscapeIsThere(t *testing.T) {
	sem := CoreSemantics()
	// `\0101` is `\010` and then a `1` — three octal digits after the
	// leading zero — which is `a \b 1 Z` in all seven columns.
	out, st := run(t, `printf 'a\tb\0101Z'`, func(r *Runner) { r.Semantics = &sem })
	if out != "a\tb\x081Z" || st != 0 {
		t.Errorf("got %q status %d, want %q and 0", out, st, "a\tb\x081Z")
	}
}

// The rest of the escape set is not this axis, and a reader that had taken
// the whole letter range would show here. Every one of these is unanimous
// across the panel and is written the same way whatever the two new axes say,
// so they are asserted with both answers.
func TestTheEscapeAxesDoNotReachTheRestOfTheFormatsEscapes(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		sem := printfSem()
		sem.PrintfEscEscape = answer
		sem.PrintfCapitalEscEscape = answer
		for _, tc := range []struct{ src, want string }{
			{`printf 'a\tb\vc\fd\re\af'`, "a\tb\vc\fd\re\af"},
			{`printf 'a\0101Z'`, "a\x081Z"},
			{`printf 'a\\eZ'`, `a\eZ`},
			{`printf 'a\gZ'`, `a\gZ`},
			{`printf 'a\FZ'`, `a\FZ`},
		} {
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want || st != 0 {
				t.Errorf("%s with the escape axes %v: got %q status %d, want %q and 0",
					tc.src, answer, out, st, tc.want)
			}
		}
	}
}
