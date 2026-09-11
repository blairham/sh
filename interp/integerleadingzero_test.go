// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// What an integer name's value reader does with a digit string that begins
// with a zero, which is not what the *arithmetic* reader in the same shell
// does with the same characters — see
// Semantics.IntegerAssignmentReadsALeadingZeroAsDecimal and #1270.
//
// Named for the axis and never for a shell, as everything in this package is.

// octalArithmetic is the base these tests share: a shell whose arithmetic
// reads a leading zero as octal, which is the only place the question arises.
func octalArithmetic(a Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ArithLeadingZeroIsOctal = Yes
		s.ArithInvalidOctalDigitIsError = Yes
		// A radix prefix teaching the name a base is a different question,
		// answered here so that the `0x10` row reaches this one.
		s.IntegerBaseComesFromTheValueAssigned = No
		s.IntegerAssignmentReadsALeadingZeroAsDecimal = a
	}
}

func TestAnIntegerAssignmentsLeadingZeroIsAnAxis(t *testing.T) {
	const src = `typeset -i d=010; echo "[$d]"`
	for _, w := range []struct {
		answer Answer
		want   string
	}{{Yes, "[10]\n"}, {No, "[8]\n"}} {
		out, errs, st := declRun(t, src, octalArithmetic(w.answer), Diagnostics{})
		if out != w.want || st != 0 || errs != "" {
			t.Errorf("%v: got %q/%d stderr %q, want %q", w.answer, out, st, errs, w.want)
		}
	}
}

// The arithmetic reader is untouched by either answer, which is the whole
// point of the axis being asked here rather than inside it: one shell reads
// the same four characters two ways.
func TestTheLeadingZeroAxisLeavesArithmeticAlone(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, _, st := declRun(t, `echo "$((010))"`, octalArithmetic(a), Diagnostics{})
		if out != "8\n" || st != 0 {
			t.Errorf("%v: got %q/%d, want the octal reading", a, out, st)
		}
	}
}

// Only a number and nothing else takes the decimal reading. Every row here is
// a shape whose two readings agree, so it must answer the same under either.
func TestOnlyAWholeZeroPaddedNumberTakesTheDecimalReading(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		// Spaces around it mean the text is not a number, so the expression
		// reader has it and the zero is octal again.
		{`typeset -i a=" 010 "; echo "[$a]"`, "[8]\n"},
		// An operator in it is an expression, and octal inside one.
		{`typeset -i b=010+1; echo "[$b]"`, "[9]\n"},
		// A prefix both readers agree about.
		{`typeset -i d=0x10; echo "[$d]"`, "[16]\n"},
		// One digit is no disagreement at all.
		{`typeset -i e=0; echo "[$e]"`, "[0]\n"},
	} {
		for _, a := range []Answer{Yes, No} {
			out, errs, st := declRun(t, tc.src, octalArithmetic(a), Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("%v %s: got %q/%d stderr %q, want %q", a, tc.src, out, st, errs, tc.want)
			}
		}
	}
}

// A sign belongs to the number, and an invalid octal digit is a digit like any
// other once the reading is decimal — the row that says the two readings are
// not the same code with a different base.
func TestTheDecimalReadingTakesASignAndAnyDigit(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i c=-010; echo "[$c]"`, "[-10]\n"},
		{`typeset -i p=+010; echo "[$p]"`, "[10]\n"},
		{`typeset -i n=09; echo "[$n]"`, "[9]\n"},
		{`typeset -i z=00; echo "[$z]"`, "[0]\n"},
	} {
		out, errs, st := declRun(t, tc.src, octalArithmetic(Yes), Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%s: got %q/%d stderr %q, want %q", tc.src, out, st, errs, tc.want)
		}
	}
	// And under the other answer the same `09` is an octal digit that does
	// not exist, which is the expression reader's refusal and not this one's.
	out, errs, _ := declRun(t, `typeset -i n=09; echo "[$n]"`, octalArithmetic(No), Diagnostics{})
	if errs == "" {
		t.Errorf("octal reading of 09: stdout %q with nothing on stderr, want a complaint", out)
	}
}

// The same reader answers a value the name is handed later and a standing
// value an attribute re-reads, because they are one function — a second
// evaluator beside it is what would drift.
func TestTheLeadingZeroReadingReachesEveryIntegerValue(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -i f; f=010; echo "[$f]"`, "[10]\n"},
		{`e=010; typeset -i e; echo "[$e]"`, "[10]\n"},
		{`typeset -i g=0; g+=010; echo "[$g]"`, "[10]\n"},
	} {
		out, errs, st := declRun(t, tc.src, func(s *Semantics) {
			octalArithmetic(Yes)(s)
			// The re-read row needs the attribute to reach backwards at all,
			// which is a different axis and is answered here so that this
			// test reaches the question it is about.
			s.AttributeRereadsTheValueItFinds = Yes
		}, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%s: got %q/%d stderr %q, want %q", tc.src, out, st, errs, tc.want)
		}
	}
}

// And where nothing made the zero octal there is nothing to choose between, so
// the axis is never asked: an unanswered field is not a refusal here.
func TestTheLeadingZeroAxisIsNotAskedWhereArithmeticIsDecimal(t *testing.T) {
	out, errs, st := declRun(t, `typeset -i d=010; echo "[$d]"`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ArithLeadingZeroIsOctal = No
		s.IntegerBaseComesFromTheValueAssigned = No
	}, Diagnostics{})
	if out != "[10]\n" || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want [10] with no refusal", out, st, errs)
	}
	// The refusal is real where the arithmetic *is* octal and nothing
	// answered, which is what says the silence above is the guard and not a
	// missing question.
	out, errs, st = declRun(t, `typeset -i d=010; echo "[$d]"`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ArithLeadingZeroIsOctal = Yes
		s.IntegerBaseComesFromTheValueAssigned = No
	}, Diagnostics{})
	if out != "[]\n" || !strings.Contains(errs, "zero-padded number assigned to an integer name") {
		t.Errorf("unanswered: got %q/%d stderr %q, want a refusal naming the axis and no value", out, st, errs)
	}
}
