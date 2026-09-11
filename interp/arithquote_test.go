// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// What a double quote inside an arithmetic expression *evaluates to*, by
// policy and never by shell. The grammar decides whether the quote is read
// through; these are the values that fall out, and they are the point of the
// flag: `$(( "$n" + 1 ))` is a shape that looks defensive and is common, and
// four of the six shells measured evaluate it (#1223).
func runQuoted(t *testing.T, p syntax.ArithDoubleQuotePolicy, src string) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArithDoubleQuote = p
	}, nil)
}

func TestADoubleQuoteInsideArithmeticEvaluates(t *testing.T) {
	for _, p := range []syntax.ArithDoubleQuotePolicy{
		syntax.ArithDoubleQuoteSkipped, syntax.ArithDoubleQuoteRemoved,
	} {
		t.Run(p.String(), func(t *testing.T) {
			src := `n=5; printf "[%s][%s][%s][%s]" "$(( "1" + 1 ))" "$(( "n" + 1 ))" ` +
				`"$(( 1 + "2" ))" "$(( "" + 7 ))"`
			out, st := runQuoted(t, p, src)
			if want := "[2][6][3][7]"; out != want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
			}
		})
	}
}

// Refused is the core's answer and the answer of the two shells that have no
// other: the quote stands where a value belongs, so the expression wants an
// operand.
func TestARefusedDoubleQuoteIsAMissingOperand(t *testing.T) {
	src := `echo $(( "1" + 1 )); echo after`
	out, st := runQuoted(t, syntax.ArithDoubleQuoteRefused, src)
	if !strings.Contains(out, "operand expected") || strings.Contains(out, "after") || st == 0 {
		t.Errorf("%s = %q (status %d), want the operand refused", src, out, st)
	}
}

// Removed is the reading that takes the bytes out before anything reads the
// text, so two digits with a quote between them are one number. Skipped is not
// that: the quote ends the token there and two operands run together.
func TestAQuoteInsideATokenPartsTheTwoReadings(t *testing.T) {
	src := `echo $(( 1"0" ))`
	if out, st := runQuoted(t, syntax.ArithDoubleQuoteRemoved, src); out != "10\n" || st != 0 {
		t.Errorf("%s removed = %q (status %d), want %q at 0", src, out, st, "10\n")
	}
	out, st := runQuoted(t, syntax.ArithDoubleQuoteSkipped, src)
	if !strings.Contains(out, "operator expected") || st == 0 {
		t.Errorf("%s skipped = %q (status %d), want the leftover refused", src, out, st)
	}
}

// The character constant is the code of the character between the quotes, and
// the escapes are the ones `$'…'` decodes rather than a second table.
func TestACharacterConstantIsACodePoint(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf "%s" $(( '1' + 1 ))`, "50"},
		{`printf "%s" $(( 'a' ))`, "97"},
		{`printf "%s" $(( 'a' + 'b' ))`, "195"},
		{`printf "%s" $(( '\n' ))`, "10"},
		{`printf "%s" $(( '\101' ))`, "65"},
		{`printf "%s" $(( '\x41' ))`, "65"},
		{`printf "%s" $(( -'a' ))`, "-97"},
		// The closing quote is optional, so the second quote is the
		// character: 39 is the code of a quote.
		{`printf "%s" $(( '' ))`, "39"},
	} {
		out, st := runGrammar(t, tc.src, func(d *syntax.Dialect) {
			d.ArithCharacterConstant = true
		}, nil)
		if out != tc.want || st != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// Nothing about one quote character predicts the other: the constant is read
// with the double quote refused, which is the pair one shell in the panel has.
func TestTheTwoQuoteQuestionsAreIndependent(t *testing.T) {
	src := `printf "[%s][%s]" "$(( 'a' ))" "$(( "1" + 1 ))"`
	out, _ := runGrammar(t, src, func(d *syntax.Dialect) {
		d.ArithCharacterConstant = true
		d.ArithDoubleQuote = syntax.ArithDoubleQuoteRefused
	}, nil)
	if !strings.HasPrefix(out, "sh:") && !strings.Contains(out, "operand expected") {
		t.Errorf("%s = %q, want the single quote read and the double one refused", src, out)
	}
	out, st := runGrammar(t, `printf "%s" $(( 'a' ))`, func(d *syntax.Dialect) {
		d.ArithCharacterConstant = true
		d.ArithDoubleQuote = syntax.ArithDoubleQuoteRefused
	}, nil)
	if out != "97" || st != 0 {
		t.Errorf("$(( 'a' )) = %q (status %d), want 97 at 0", out, st)
	}
}
