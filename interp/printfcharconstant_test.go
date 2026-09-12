// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// `printf '%d' "'A"` is 65 — a numeric conversion given an operand that
// begins with a quote takes the *character's* value instead of reading
// digits. POSIX XCU says so in so many words and all seven columns have it,
// so it is the core's rather than a dialect's.
//
// It is also the only way a shell has of asking "what is the code point of
// this character", which is what made the gap sting: `printf '0x%x' "'a"`
// answered `printf: 'a: invalid number` and printed `0x0`.

// charConst runs src with every printf axis answered and returns what came
// out with the status.
func charConst(t *testing.T, src string, vars map[string]string) (string, int) {
	t.Helper()
	sem := printfSem()
	sem.PrintfReportsBadNumber = Yes
	sem.MultibyteEncodingIsHonored = Yes
	return run(t, src, func(r *Runner) {
		r.Semantics = &sem
		r.Vars = vars
	})
}

func TestAQuotedOperandIsTheCharactersValue(t *testing.T) {
	for _, tc := range []struct {
		src, want string
	}{
		{`printf '%d' "'A"`, "65"},
		// A double quote does the same job as a single one.
		{`printf '%d' '"A'`, "65"},
		{`printf '0x%x' "'a"`, "0x61"},
		{`printf '%o' "'A"`, "101"},
		// Widened for a float conversion rather than refused.
		{`printf '%f' "'A"`, "65.000000"},
		{`printf '%5.2f' "'A"`, "65.00"},
		// Everything past the first character is ignored.
		{`printf '%d' "'AB"`, "65"},
		// A quote with nothing after it is zero, and not an error.
		{`printf '%d' "'"`, "0"},
		// `%c` is not a numeric conversion, so the quote is just a
		// character there: it prints the quote itself.
		{`printf '%c' "'z"`, "'"},
	} {
		out, st := charConst(t, tc.src, nil)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q and 0", tc.src, out, st, tc.want)
		}
	}
}

// TestTheQuoteHasToBeTheFirstCharacter. A blank in front of it makes the word
// an ordinary operand again — six of the seven columns report a bad number,
// and it matters because the operand is *not* trimmed before this reading
// where a plain numeral is.
func TestTheQuoteHasToBeTheFirstCharacter(t *testing.T) {
	out, st := charConst(t, `printf '%d' " 'A"`, nil)
	if out != "sh: printf:  'A: invalid number\n0" || st != 1 {
		t.Errorf(`printf '%%d' " 'A" = %q status %d, want the complaint, a zero, and 1`, out, st)
	}
}

// TestTheCharacterIsTheLocalesOwn, through the same reader every other length
// and position in this package goes through: a UTF-8 locale gives the code
// point and a single-byte one gives the first byte. Both readings are the
// panel's — `printf '%d' "'é"` is 233 under a UTF-8 locale and 195 under
// LC_ALL=C, in bash and zsh alike.
func TestTheCharacterIsTheLocalesOwn(t *testing.T) {
	for _, tc := range []struct {
		locale, want string
	}{
		{"en_US.UTF-8", "233"},
		{"C", "195"},
	} {
		out, st := charConst(t, `printf '%d' "'é"`, map[string]string{"LC_ALL": tc.locale})
		if out != tc.want || st != 0 {
			t.Errorf("LC_ALL=%s: got %q status %d, want %q and 0", tc.locale, out, st, tc.want)
		}
	}
}
