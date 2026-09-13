// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A `*` or `@` subscript inside an arithmetic expression: the slice the same
// subscript takes in an expansion, or an ordinary subscript that is not a
// number.

func arithSlice(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := CoreSemantics()
		if r.Semantics != nil {
			s = *r.Semantics
		}
		s.ArithWholeArraySubscriptIsTheSlice = a
		// The *left* of an assignment is a different question with its own
		// pair of fields, and one setup line here writes `m[*]=4` to make a
		// key for the expression to read. Answered the way the column that
		// stores a key does, so this suite is about the expression and not
		// about the store — see Semantics.WholeArraySubscriptAssigningATable.
		s.WholeArraySubscriptAssigningATable = WholeArraySubscriptIsAnOrdinaryKey
		r.Semantics = &s
	}
}

func TestAWholeArraySubscriptInAnExpressionAnswersByAxis(t *testing.T) {
	for _, tc := range []struct{ name, src, slice string }{
		{"a table under `*`", `typeset -A m; m[k]=9; echo $(( m[*] ))`, "9"},
		{"a table under `@`", `typeset -A m; m[k]=9; echo $(( m[@] ))`, "9"},
		{"an array of one", `a=(3); echo $(( a[*] + 1 ))`, "4"},
		{"an empty array", `a=(); echo $(( a[*] ))`, "0"},
		{"a scalar", `s=7; echo $(( s[*] ))`, "7"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, subscripting, arithSlice(Yes))
			if got := strings.TrimSpace(out); got != tc.slice || status != 0 {
				t.Errorf("the slice: %s = %q (status %d), want %q at 0", tc.src, got, status, tc.slice)
			}
			// The other answer reads the brackets as a subscript, where `*`
			// is a key an association has not got and no number at all on an
			// array — so the row is not the slice, whatever else it is.
			out, _ = runGrammar(t, tc.src, subscripting, arithSlice(No))
			if got := strings.TrimSpace(out); got == tc.slice && tc.slice != "0" {
				t.Errorf("the subscript reading: %s = %q, want anything but the slice", tc.src, got)
			}
		})
	}
}

// The join is the first character of IFS, and `@` joins the same way `*` does
// — there is no field splitting inside an expression for the two to part
// over, which is what keeps this off the unquoted-`@` axis.
func TestTheArithmeticSliceJoinsOnIFS(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a=(3 4); IFS=; echo $(( a[*] ))`, "34"},
		{`a=(3 4); IFS=; echo $(( a[@] ))`, "34"},
	} {
		out, status := runGrammar(t, tc.src, subscripting, arithSlice(Yes))
		if got := strings.TrimSpace(out); got != tc.want || status != 0 {
			t.Errorf("%s = %q (status %d), want %q at 0", tc.src, got, status, tc.want)
		}
	}
}

// A slice of more than one element is usually a failure rather than a number,
// which is the answer and not a defect in it: the joined text is read as an
// expression, and `3 4 5` is not one.
func TestASliceOfSeveralElementsFailsAsAnExpression(t *testing.T) {
	out, status := runGrammar(t, `a=(3 4 5); echo $(( a[*] ))`, subscripting, arithSlice(Yes))
	if status == 0 || strings.TrimSpace(out) == "3" {
		t.Errorf("a=(3 4 5); $(( a[*] )) = %q (status %d), want the expression to fail", out, status)
	}
}

// The slice is read *before* the association, because the key `*` is exactly
// what the other answer makes of the same text: a table consulted first would
// answer the key and the two readings would collapse into one.
func TestTheSliceIsAskedBeforeTheKey(t *testing.T) {
	const src = `typeset -A m; m[k]=9; m[*]=4; echo $(( m[*] ))`
	out, _ := runGrammar(t, src, subscripting, arithSlice(No))
	if got := strings.TrimSpace(out); got != "4" {
		t.Errorf("the subscript reading: %s = %q, want the key's value", src, got)
	}
	out, _ = runGrammar(t, src, subscripting, arithSlice(Yes))
	if got := strings.TrimSpace(out); got == "4" {
		t.Errorf("the slice: %s = %q, want the slice rather than the key", src, got)
	}
}

// arithBadWholeArray answers the report axis, leaving the slice one alone.
func arithBadWholeArray(a Answer) func(*Runner) {
	return func(r *Runner) {
		s := CoreSemantics()
		if r.Semantics != nil {
			s = *r.Semantics
		}
		s.ArithWholeArraySubscriptIsTheSlice = No
		s.ArithWholeArraySubscriptIsReportedAsBad = a
		r.Semantics = &s
	}
}

// Where the brackets are not the slice, one column *reports* them and answers
// zero — so the expression survives — and the other lets the arithmetic refuse
// the whole thing.
//
// The two agree about the value once the message is past, which is why the
// report and the survival are one axis and not a wording (#1978).
func TestAWholeArraySubscriptMayBeReportedAndAnsweredZero(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"an array", `a=(3 4 5); echo $(( a[*] ))`, "0"},
		{"the star as an operand", `a=(3); echo $(( a[*] + 1 ))`, "1"},
		{"and the at sign", `a=(3); echo $(( a[@] + 1 ))`, "1"},
		{"a name nothing declared", `echo $(( nodecl[*] ))`, "0"},
		{"a scalar", `s=7; echo $(( s[*] ))`, "0"},
		{"in the middle of an expression", `a=(3); echo $(( 1 + a[*] + 1 ))`, "2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, status := runGrammar(t, tc.src, subscripting, arithBadWholeArray(Yes))
			if !strings.Contains(out, "bad array subscript") {
				t.Errorf("%s = %q, want the subscript named", tc.src, out)
			}
			if !strings.Contains(out, "\n"+tc.want+"\n") && !strings.HasSuffix(out, tc.want+"\n") {
				t.Errorf("%s = %q, want the expression to answer %q", tc.src, out, tc.want)
			}
			if status != 0 {
				t.Errorf("%s: status %d, want 0 — the expression survives", tc.src, status)
			}
			// The other answer abandons the expression instead, which is the
			// reading a subscript that is no number has always had.
			out, status = runGrammar(t, tc.src, subscripting, arithBadWholeArray(No))
			if status == 0 {
				t.Errorf("refusing: %s = %q (status 0), want the expression abandoned", tc.src, out)
			}
		})
	}
}

// The subject is the name and the subscript as written.
func TestAReportedWholeArraySubscriptNamesBoth(t *testing.T) {
	out, _ := runGrammar(t, `a=(3); echo $(( a[@] ))`, subscripting, arithBadWholeArray(Yes))
	if !strings.Contains(out, "a[@]: bad array subscript") {
		t.Errorf("= %q, want `a[@]` named", out)
	}
}

// An **association** never reaches the question: the brackets are the key `*`,
// nothing is stored under it, and the answer is a silent zero in both columns.
func TestAWholeArraySubscriptOnATableAsksNothing(t *testing.T) {
	for _, a := range []Answer{Yes, No, Unspecified} {
		const src = `typeset -A m; m[k]=9; echo $(( m[*] ))`
		out, status := runGrammar(t, src, subscripting, arithBadWholeArray(a))
		if strings.TrimSpace(out) != "0" || status != 0 {
			t.Errorf("answer %v: %s = %q (status %d), want a silent 0", a, src, out, status)
		}
	}
}

// The spelling has to be exact. `$(( a[ * ] ))` is an arithmetic syntax error
// in every column measured — the brackets are the whole-array spelling only
// when they hold the one character and nothing else — and trimming answered a
// different question, and answered it wrongly for the slice column too.
func TestABlankWholeArraySubscriptIsNotTheSpelling(t *testing.T) {
	for _, setup := range []func(*Runner){arithSlice(Yes), arithBadWholeArray(Yes)} {
		const src = `a=(3 4); echo $(( a[ * ] ))`
		out, status := runGrammar(t, src, subscripting, setup)
		if status == 0 || strings.Contains(out, "bad array subscript") {
			t.Errorf("%s = %q (status %d), want the arithmetic to refuse it", src, out, status)
		}
	}
}

// The write reports too and stores nothing, and it is *not* an error: measured,
// `(( a[*] = 5 ))` ends at 0 with every element as it was, and `(( a[*]++ ))`
// writes the sentence twice — once for the read and once for the store.
func TestAWholeArraySubscriptIsReportedOnTheWriteToo(t *testing.T) {
	src := `a=(3 4); (( a[*] = 5 )); echo "st=$?"; printf "[%s]" "${a[@]}"`
	out, _ := runGrammar(t, src, subscripting, arithBadWholeArray(Yes))
	if !strings.Contains(out, "st=0") || !strings.Contains(out, "[3][4]") {
		t.Errorf("%s = %q, want the report, status 0 and the array untouched", src, out)
	}
	src = `a=(3 4); (( a[*]++ )); printf "[%s]" "${a[@]}"`
	out, _ = runGrammar(t, src, subscripting, arithBadWholeArray(Yes))
	if n := strings.Count(out, "bad array subscript"); n != 2 {
		t.Errorf("%s = %q, want the sentence twice, got %d", src, out, n)
	}
	if !strings.Contains(out, "[3][4]") {
		t.Errorf("%s = %q, want the array untouched", src, out)
	}
}
