// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What the word-spelled comparisons of `test` and `[` read their operands as,
// and what a condition operand's leading zeros hide (#1626, #1627).
//
// The branch a snippet took is read with lastLine: the wordings here are the
// core's and each dialect writes its own, so what these tests pin is the
// answer and not the sentence written ahead of it.
//
// The two are one question asked at two sites. A dialect that reads an operand
// as arithmetic reads it the way `[[ ]]` already does, so the second axis
// below — how far the leading-zero rewrite reaches — is answered once and seen
// in both constructs.

// comparisonRun runs one snippet with the two comparison readings answered by
// hand, and with the double-bracket grammar on so that both constructs are
// reachable from one source.
func comparisonRun(t *testing.T, src string, set func(*Semantics)) (string, int) {
	t.Helper()
	return runGrammar(t, src, func(d *syntax.Dialect) { d.DoubleBracket = true }, func(r *Runner) {
		sem := CoreSemantics()
		sem.ArithLeadingZeroIsOctal = Yes
		sem.ArithInvalidOctalDigitIsError = No
		sem.ArithNameValueRecurses = Yes
		sem.ArithStoredValueReadsALeadingZeroAsDecimal = No
		sem.TestBuiltinComparisonOperandsAreArithmetic = No
		sem.ConditionArithmeticErrorIsFatal = No
		set(&sem)
		r.Semantics = &sem
	})
}

// A numeral is a numeral under either reading, so the axis has a side that
// changes nothing — and the cases that separate them are a name, an operator
// and an empty word.
func TestComparisonOperandsCanBeArithmetic(t *testing.T) {
	for _, tc := range []struct {
		name, src, yes, no string
	}{
		{
			"a name is its value",
			`n=5; [ n -eq 5 ] && echo same || echo differs`,
			"same", "differs",
		},
		{
			"an expression is evaluated",
			`[ 1+1 -eq 2 ] && echo same || echo differs`,
			"same", "differs",
		},
		{
			"nothing is zero",
			`[ "" -eq 0 ] && echo same || echo differs`,
			"same", "differs",
		},
		{
			// The reading is the whole language, side effects included.
			"an assignment written in an operand lands",
			`n=5; [ "n=9" -eq 9 ] >/dev/null; echo "n=$n"`,
			"n=9", "n=5",
		},
		{
			// Both readings answer this one the same way, which is why the
			// axis is not asked for it at all.
			"two plain numerals need no reading",
			`[ 7 -eq 7 ] && echo same || echo differs`,
			"same", "same",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := comparisonRun(t, tc.src, func(s *Semantics) {
				s.TestBuiltinComparisonOperandsAreArithmetic = Yes
			})
			if got := lastLine(out); got != tc.yes {
				t.Errorf("arithmetic: got %q, want %q", got, tc.yes)
			}
			out, _ = comparisonRun(t, tc.src, func(s *Semantics) {
				s.TestBuiltinComparisonOperandsAreArithmetic = No
			})
			if got := lastLine(out); got != tc.no {
				t.Errorf("numeral: got %q, want %q", got, tc.no)
			}
		})
	}
}

// A refusal on the arithmetic side is loud, is status 1 rather than the
// not-an-expression 2, and lets the script run on — where the numeral side
// says "integer expected" at 2. The name the builtin was called by opens
// either sentence.
func TestComparisonArithmeticRefusalIsTheBuiltins(t *testing.T) {
	src := `[ 1x1 -eq 0 ]; echo "st=$?"; test 1x1 -eq 0; echo "st=$?"`
	out, _ := comparisonRun(t, src, func(s *Semantics) {
		s.TestBuiltinComparisonOperandsAreArithmetic = Yes
	})
	for _, want := range []string{"[: ", "test: ", "1x1", "st=1"} {
		if !strings.Contains(out, want) {
			t.Errorf("arithmetic refusal %q lacks %q", out, want)
		}
	}
	if strings.Count(out, "st=1") != 2 {
		t.Errorf("arithmetic refusal %q, want both constructs at 1", out)
	}
	if strings.Contains(out, "st=2") {
		t.Errorf("arithmetic refusal %q ended at 2, want 1", out)
	}
	out, _ = comparisonRun(t, src, func(s *Semantics) {
		s.TestBuiltinComparisonOperandsAreArithmetic = No
	})
	if !strings.Contains(out, "integer expected") || !strings.Contains(out, "st=2") {
		t.Errorf("numeral refusal %q, want the integer complaint at 2", out)
	}
}

// The reading a division by zero gets is the arithmetic's, not the builtin's:
// the sentence is the one `$(( ))` writes for it, behind the builtin's name.
func TestComparisonArithmeticCarriesTheMathComplaint(t *testing.T) {
	out, st := comparisonRun(t, `[ 3/0 -eq 0 ]; echo "st=$?"`, func(s *Semantics) {
		s.TestBuiltinComparisonOperandsAreArithmetic = Yes
	})
	if !strings.Contains(out, "division by zero") || !strings.Contains(out, "st=1\n") {
		t.Errorf("out=%q st=%d, want the division reported at 1", out, st)
	}
}

// How far the leading-zero rewrite reaches, which is the other half of #1627.
//
// A *stored* value keeps its `0x` prefix and a *condition operand* does not,
// so the same six characters are sixteen in one place and a name in the other.
// Both sit behind ArithStoredValueReadsALeadingZeroAsDecimal: where nothing
// rewrites a leading zero the two sites cannot differ.
func TestConditionOperandZerosReachPastARadixPrefix(t *testing.T) {
	const src = `x10=7
echo "stored=$(( 0x10 ))"
[[ 0x10 -eq 7 ]] && echo cond=name || echo cond=hex
[ 0x10 -eq 7 ] && echo test=name || echo test=hex
[[ 1+0x10 -eq 17 ]] && echo inner=hex || echo inner=name`
	out, _ := comparisonRun(t, src, func(s *Semantics) {
		s.ArithStoredValueReadsALeadingZeroAsDecimal = Yes
		s.TestBuiltinComparisonOperandsAreArithmetic = Yes
	})
	want := "stored=16\ncond=name\ntest=name\ninner=hex\n"
	if out != want {
		t.Errorf("rewriting: got %q, want %q", out, want)
	}
	out, _ = comparisonRun(t, src, func(s *Semantics) {
		s.ArithStoredValueReadsALeadingZeroAsDecimal = No
		s.TestBuiltinComparisonOperandsAreArithmetic = Yes
	})
	want = "stored=16\ncond=hex\ntest=hex\ninner=hex\n"
	if out != want {
		t.Errorf("not rewriting: got %q, want %q", out, want)
	}
}

// And the rest of the rewrite's edges, which the same answer decides at both
// sites: however many zeros, in front of a name as much as in front of a
// digit, and two zeros in front of an `x` are not a prefix.
func TestLeadingZerosComeOffAValueAndAnOperand(t *testing.T) {
	const src = `abc=5; b101=9; x10=7
name=0abc; binary=0b101; padded=0010; twice=00x10
echo "name=$(( name )) binary=$(( binary )) padded=$(( padded )) twice=$(( twice ))"
[[ 010 -eq 10 ]] && echo dec || echo oct
[[ 0010#5 -eq 5 ]] && echo based || echo nobase`
	out, _ := comparisonRun(t, src, func(s *Semantics) {
		s.ArithStoredValueReadsALeadingZeroAsDecimal = Yes
	})
	want := "name=5 binary=9 padded=10 twice=7\ndec\nbased\n"
	if out != want {
		t.Errorf("rewriting: got %q, want %q", out, want)
	}
	out, _ = comparisonRun(t, src, func(s *Semantics) {
		s.ArithStoredValueReadsALeadingZeroAsDecimal = No
	})
	if strings.Contains(out, "name=5") {
		t.Errorf("not rewriting: got %q, want the zeros left on", out)
	}
}
