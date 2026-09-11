// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A subscript holding whitespace and nothing else is what `a[$w]` is once a
// `$w` holding spaces has gone in, because an arithmetic expansion substitutes
// its parameters before it parses. It is tested here by axis and never by
// shell: the question is what the blank expression means where a subscript
// wants a value, and the panel gives two answers (#1762).
func blankSub(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := *r.Semantics
		s.BlankArithSubscriptIsTheEmptyExpression = a
		r.Semantics = &s
	}
}

// Yes reads the brackets as the blank expression, which is zero — so the
// operand is the *element* that names and not the number. The array is what
// carries that distinction: against an unset name both answers are zero.
func TestABlankArithmeticSubscriptAnswersByAxis(t *testing.T) {
	for _, tc := range []struct {
		name   string
		a      Answer
		src    string
		want   string
		status int
	}{
		{
			"the blank expression names element zero",
			Yes,
			`a=(5 6 7); echo $(( a[ ] )); echo after`,
			"5\nafter\n", 0,
		},
		{
			"refused, and the expression produces nothing",
			No,
			`a=(5 6 7); echo $(( a[ ] )); echo after`,
			"sh:  : operand expected\n", 2,
		},
		{
			"a subscript of spaces is still the blank expression",
			Yes,
			`a=(5 6 7); w="   "; echo $(( a[$w] )); echo after`,
			"5\nafter\n", 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := run(t, tc.src, blankSub(tc.a))
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q (status %d), want %q at %d",
					tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The blank text and the empty text are two axes, and this is the pair that
// says so: one dialect reads the blank brackets as element zero and reports
// the empty ones. An axis worded for both would have to give one of those
// answers to the other.
func TestABlankSubscriptIsNotAnEmptySubscript(t *testing.T) {
	src := `a=(5 6 7); echo $(( a[ ] )); echo $(( a[] ))`
	out, st := run(t, src, func(r *Runner) {
		s := *r.Semantics
		s.BlankArithSubscriptIsTheEmptyExpression = Yes
		s.EmptyArithSubscript = EmptyArithSubscriptIsReported
		r.Semantics = &s
	})
	if want := "5\nsh: a[]: bad array subscript\n0\n"; out != want || st != 0 {
		t.Errorf("%s = %q (status %d), want %q at 0", src, out, st, want)
	}
}

// An associative name's subscript is a key and never an expression, so the
// axis is not reached there at all — whitespace between the brackets is a key
// made of spaces.
//
// Unspecified is the probe that says "not reached" rather than "reached and
// permitted": an axis that was consulted with no answer would refuse by name.
func TestABlankSubscriptOnAnAssociativeNameIsAKey(t *testing.T) {
	for _, a := range []Answer{Yes, No, Unspecified} {
		src := `typeset -A m; k=" "; m[$k]=7; echo $(( m[ ] )); echo after`
		out, st := run(t, src, blankSub(a))
		if want := "7\nafter\n"; out != want || st != 0 {
			t.Errorf("with the axis %v, %s = %q (status %d), want %q at 0",
				a, src, out, st, want)
		}
	}
}

// The same split, unchanged, with the subscript standing as an assignment
// target. The guard is load-bearing rather than defensive: the write falls
// through to the *bare name* when there is no index, so a refused
// `(( a[ ] = 9 ))` would otherwise overwrite the array with a number.
func TestABlankSubscriptAsAnAssignmentTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		a    Answer
		want string
	}{
		{"permitted: element zero is written", Yes, "9 6 7"},
		{"refused: the array is left alone", No, "5 6 7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := `a=(5 6 7); (( a[ ] = 9 )); printf "%s" "${a[*]}"; echo`
			out, _ := run(t, src, blankSub(tc.a))
			if got := lastLine(out); got != tc.want {
				t.Errorf("%s left %q, want %q", src, got, tc.want)
			}
		})
	}
}

// No answer is refused by name. The two that exist leave different values and
// different streams behind, so neither can stand in for the other.
func TestABlankArithmeticSubscriptUnansweredIsRefused(t *testing.T) {
	out, st := run(t, `a=(5 6 7); echo $(( a[ ] )); echo after`, blankSub(Unspecified))
	if !strings.Contains(out, "a subscript holding only whitespace") ||
		!strings.Contains(out, "no dialect was chosen") {
		t.Errorf("got %q, want the axis named in the refusal", out)
	}
	if strings.Contains(out, "after") || st == 0 {
		t.Errorf("got %q (status %d), want the input abandoned", out, st)
	}
}

// The axis is asked at the subscript and not at the expression, which is where
// the panel actually splits: an expansion holding nothing but spaces is zero
// in every shell that has subscripts at all, so an axis placed there would
// move a row the panel agrees about.
func TestABlankExpressionIsNotAffectedByTheSubscriptAxis(t *testing.T) {
	for _, a := range []Answer{Yes, No, Unspecified} {
		src := `echo $((   )); echo after`
		out, st := run(t, src, blankSub(a))
		if want := "0\nafter\n"; out != want || st != 0 {
			t.Errorf("with the axis %v, %s = %q (status %d), want %q at 0",
				a, src, out, st, want)
		}
	}
}
