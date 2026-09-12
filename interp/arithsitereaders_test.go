// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A leading zero is read by four different pieces of code — the arithmetic
// lexer, a value dereferenced into an expression, a `let` word and a word
// comparison's operand — and one shell answers three different ways across
// them. These are the two sites that were reading it as the lexer does.

func leadingZeroSemantics(letDecimal, storedDecimal Answer) Semantics {
	s := testSemantics()
	s.ArithLeadingZeroIsOctal = Yes
	s.ArithInvalidOctalDigitIsError = Yes
	s.ArithStoredValueReadsALeadingZeroAsDecimal = storedDecimal
	s.LetReadsALeadingZeroAsDecimal = letDecimal
	s.ArithNameValueRecurses = Yes
	return s
}

// LetReadsALeadingZeroAsDecimal, and the reach that separates it from the
// stored value's rewrite: `let` turns the octal rule off for the whole word,
// so every numeral in it goes decimal, where a value's *leading* numeral is
// rewritten and the rest stays octal. `1+010` is the probe that parts them.
func TestLetHasAReaderOfItsOwnForALeadingZero(t *testing.T) {
	const src = `let "x=010"; let "y=1+010"; echo "[$x][$y][$(( 010 ))]"`
	if out, st := run(t, src, withSem(leadingZeroSemantics(Yes, Yes))); strings.TrimSpace(out) != "[10][11][8]" || st != 0 {
		t.Errorf("decimal: got %q (status %d), want [10][11][8] at 0", out, st)
	}
	// And with the same reader as `(( ))`, which is what the other three do.
	if out, st := run(t, src, withSem(leadingZeroSemantics(No, Yes))); strings.TrimSpace(out) != "[8][9][8]" || st != 0 {
		t.Errorf("octal: got %q (status %d), want [8][9][8] at 0", out, st)
	}
}

// It lasts exactly as long as the builtin: a Runner is shared, so a `let` that
// turned the octal rule off for good would silently move every expression
// after it.
func TestLetsReaderDoesNotOutliveTheBuiltin(t *testing.T) {
	out, st := run(t, `let "x=010"; echo "[$x][$(( 010 ))]"`, withSem(leadingZeroSemantics(Yes, Yes)))
	if strings.TrimSpace(out) != "[10][8]" || st != 0 {
		t.Errorf("got %q (status %d), want [10][8] at 0", out, st)
	}
}

// The word comparison's operand is the *stored value's* reader and not a
// fourth answer — a bare operand goes decimal and a numeral standing later in
// an expression does not, which is the leading-numeral rule exactly.
func TestAWordComparisonOperandReadsALeadingZeroLikeAStoredValue(t *testing.T) {
	const src = `[[ 010 -eq 10 ]] && echo ten; [[ 010 -eq 8 ]] && echo eight; ` +
		`[[ 1+010 -eq 9 ]] && echo nine; [[ 1+010 -eq 11 ]] && echo eleven`
	out, _ := runGrammar(t, src,
		func(d *syntax.Dialect) { d.DoubleBracket = true },
		withSem(leadingZeroSemantics(No, Yes)))
	if got := strings.Fields(out); len(got) != 2 || got[0] != "ten" || got[1] != "nine" {
		t.Errorf("decimal: got %q, want ten and nine", out)
	}
	out, _ = runGrammar(t, src,
		func(d *syntax.Dialect) { d.DoubleBracket = true },
		withSem(leadingZeroSemantics(No, No)))
	if got := strings.Fields(out); len(got) != 2 || got[0] != "eight" || got[1] != "nine" {
		t.Errorf("octal: got %q, want eight and nine", out)
	}
}

// ArithmeticAssignmentDeclaresAnInteger, and the half of it a fresh-shell
// probe cannot see: it is a *declaration* the arithmetic makes, so it reaches
// only a name that was not there before.
func TestAnArithmeticAssignmentDeclaresAnIntegerOnlyWhenItCreatesTheName(t *testing.T) {
	declaring := func(a Answer) Semantics {
		s := testSemantics()
		s.ArithmeticAssignmentDeclaresAnInteger = a
		s.IntegerBaseComesFromTheValueAssigned = Yes
		return s
	}
	for _, c := range []struct{ name, src, yes, no string }{
		{"a name it creates", `(( x = 5 )); x=2+3; echo "[$x]"`, "[5]", "[2+3]"},
		{"one that already exists", `x=3; (( x = 5 )); x=2+3; echo "[$x]"`, "[2+3]", "[2+3]"},
		{"one holding nothing", `x=; (( x = 5 )); x=2+3; echo "[$x]"`, "[2+3]", "[2+3]"},
		{"a step that creates it", `(( n++ )); n=2+3; echo "[$n]"`, "[5]", "[2+3]"},
		{"a for header", `for (( i=0; i<2; i++ )); do :; done; i=2+3; echo "[$i]"`, "[5]", "[2+3]"},
		{"a let word", `let "z = 3"; z=2+3; echo "[$z]"`, "[5]", "[2+3]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := run(t, c.src, withSem(declaring(Yes))); strings.TrimSpace(out) != c.yes {
				t.Errorf("declaring: got %q, want %q", out, c.yes)
			}
			if out, _ := run(t, c.src, withSem(declaring(No))); strings.TrimSpace(out) != c.no {
				t.Errorf("not declaring: got %q, want %q", out, c.no)
			}
		})
	}
}

// And the base comes with the attribute, from a radix the *expression* was
// written with — the answer is decimal by the time it is stored, so the text
// cannot carry it. Asserted through what a later plain assignment reads back,
// which is where the base is visible rather than only listed.
func TestAnArithmeticDeclarationTakesItsBaseFromTheExpression(t *testing.T) {
	sem := testSemantics()
	sem.ArithmeticAssignmentDeclaresAnInteger = Yes
	sem.IntegerBaseComesFromTheValueAssigned = Yes
	sem.ArithNameValueRecurses = Yes
	// The alphabet a base is spelled in, which is what makes a learned base
	// visible at all — a dialect that spells none records none.
	sem.IntegerBaseDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	sem.IntegerAttributeTakesABase = Yes
	for _, c := range []struct{ name, src, want string }{
		{"a radix literal", `(( y = 0x1f )); echo "[$y]"`, "[16#1F]"},
		{"one standing in an expression", `(( y = 1 + 0x1f )); echo "[$y]"`, "[16#20]"},
		{"a named base", `(( y = 8#7 )); echo "[$y]"`, "[8#7]"},
		// The prefix arrived through a value, so there is no literal here to
		// learn from and the name is an integer with no base.
		{"a radix reached through a value", `w=0x1f; (( y = w )); echo "[$y]"`, "[31]"},
		{"no radix at all", `(( y = 31 )); echo "[$y]"`, "[31]"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := run(t, c.src, withSem(sem)); strings.TrimSpace(out) != c.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}
