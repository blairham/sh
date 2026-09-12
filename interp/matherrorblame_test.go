// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// How much of an expression a math complaint blames.
//
// The reason and the status were already right; what differed was how much
// text the sentence quoted. Two rules, and which applies is decided by where
// the failure was raised — measured 2026-09-12 on bash 5.3.15, the one column
// in the panel that names a token at all:
//
//	$(( 1/0 + 2 ))   1/0 + 2 : division by 0 (error token is "0 + 2 ")
//	$(( 8#9 + 1 ))   8#9: value too great for base (error token is "8#9")
//
// An *evaluation* failure names the text from the failing operand to the end
// of the expression, and a literal the number reader refused names the literal
// alone with the expression stopping there. Ours named the operand alone in
// both (#2009), and dropped the trailing blank from the token besides (#1505).

// blameDiags words a complaint so both halves are visible: the expression it
// quotes, and the token it blames inside it.
func blameDiags() Diagnostics {
	return Diagnostics{
		ArithError:                  "%[1]s: %[2]s [%[3]s]",
		ArithErrorNamesThePrefix:    true,
		ArithErrorSkipsLeadingSpace: true,
		DivisionByZero:              "division by 0",
	}
}

func blameSetup(d Diagnostics) func(*Runner) {
	s := testSemantics()
	return func(r *Runner) {
		r.Semantics = &s
		r.Diagnostics = &d
	}
}

// TestAnEvaluationFailureBlamesToTheEndOfTheExpression pins the first rule.
func TestAnEvaluationFailureBlamesToTheEndOfTheExpression(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=$(( 1/0 ))`, `1/0 : division by 0 [0 ]`},
		{`x=$(( 1/0 + 2 ))`, `1/0 + 2 : division by 0 [0 + 2 ]`},
		{`x=$(( 5%0 ))`, `5%0 : division by 0 [0 ]`},
		{`x=$(( 1/0/2 ))`, `1/0/2 : division by 0 [0/2 ]`},
		// The divisor as *written*, which is why the offset has to come from
		// the parser: the tree records what an operand is, and a
		// parenthesised one has no text of its own to name.
		{`x=$(( 1/(0) ))`, `1/(0) : division by 0 [(0) ]`},
		{`x=$(( 1/((0)) ))`, `1/((0)) : division by 0 [((0)) ]`},
		{`x=$(( 1/(0+0) ))`, `1/(0+0) : division by 0 [(0+0) ]`},
		{`x=$(( 1/-0 ))`, `1/-0 : division by 0 [-0 ]`},
		// Two identical operands, which is what says the offset is read and
		// not the token's first occurrence: the blame is the divisor, and the
		// dividend is spelled the same way.
		{`x=$(( 0/0 ))`, `0/0 : division by 0 [0 ]`},
	} {
		out, st := run(t, tc.src, blameSetup(blameDiags()))
		if st == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: = %q, want %q in it", tc.src, out, tc.want)
		}
	}
}

// TestALiteralTheReaderRefusedBlamesItself is the other half, and the control:
// the same dialect stops the expression at the literal instead of running the
// blame on to the end. Without it the change above reads as "quote more", and
// what it actually is is "quote more *here*".
func TestALiteralTheReaderRefusedBlamesItself(t *testing.T) {
	d := blameDiags()
	d.DigitTooGreatForBase = "value too great for base"
	for _, tc := range []struct{ src, want string }{
		{`x=$(( 8#9 ))`, `8#9: value too great for base [8#9]`},
		{`x=$(( 8#9 + 1 ))`, `8#9: value too great for base [8#9]`},
	} {
		out, st := run(t, tc.src, blameSetup(d))
		if st == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: = %q, want %q in it", tc.src, out, tc.want)
		}
	}
}

// TestAnOperandThatRanOutIsBlamedFromItsOperator is the parse-side half of the
// same rule, and the one #1505 named: the token was the operator alone where
// the text after it belongs to the blame too, so `$(( 1 + ))` said `"+"` where
// the expression it quoted ended in a blank.
//
// The two halves were trimmed in opposite directions, which is what made it
// two bugs rather than an off-by-one: the leading blank came off the
// expression and the trailing one came off the token.
func TestAnOperandThatRanOutIsBlamedFromItsOperator(t *testing.T) {
	d := blameDiags()
	for _, tc := range []struct{ src, want string }{
		{`x=$(( 1 + ))`, `1 + : `},
		{`x=$(( 1 + ))`, `[+ ]`},
		{`x=$((1+))`, `[+]`},
		{`x=$(( x= ))`, `[= ]`},
		{`x=$(( - ))`, `[- ]`},
	} {
		out, st := run(t, tc.src, blameSetup(d))
		if st == 0 {
			t.Errorf("%s: status 0, want a failure", tc.src)
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: = %q, want %q in it", tc.src, out, tc.want)
		}
	}
}

// TestAComplaintQuotesTheTextTheTreeWasBuiltFrom is the seam an expansion
// crosses. `(( 1/$d ))` with d=0 is an expression nobody wrote down: the tree
// comes from the *expanded* text and the complaint quoted the unexpanded one,
// so a shell that blames an offset inside it blamed a different string from
// the one it was slicing.
func TestAComplaintQuotesTheTextTheTreeWasBuiltFrom(t *testing.T) {
	for _, src := range []string{
		`d=0; (( 1/$d ))`,
		`d=0; x=$(( 1/$d ))`,
	} {
		out, _ := run(t, src, blameSetup(blameDiags()))
		if !strings.Contains(out, `1/0 : division by 0 [0 ]`) {
			t.Errorf("%s: = %q, want the expanded expression quoted", src, out)
		}
		if strings.Contains(out, `$d`) {
			t.Errorf("%s: = %q, want the text the tree was built from", src, out)
		}
	}
}

// TestAnOperandKeepsTheBlanksItWasWrittenWith is #2010: a condition operand
// and a subscript reached the complaint already trimmed, so the column that
// quotes an expression back exactly could not say the blanks had been there.
//
// The dialect here does *not* skip the leading blanks, which is what makes the
// row visible — the one that skips them agrees either way.
func TestAnOperandKeepsTheBlanksItWasWrittenWith(t *testing.T) {
	d := Diagnostics{ArithError: "%[1]s: %[2]s", DivisionByZero: "divide by zero"}
	for _, tc := range []struct{ src, want string }{
		{`[[ " 1/0 " -eq 1 ]]`, ` 1/0 : divide by zero`},
		{`a=(1 2); echo ${a[ 1/0 ]}`, ` 1/0 : divide by zero`},
	} {
		out, _ := run(t, tc.src, blameSetup(d))
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: = %q, want %q in it", tc.src, out, tc.want)
		}
	}
	// And the trim it must not have taken away: an operand that is nothing
	// but blanks is still zero, answered before the reader is asked for a
	// primary it has none for.
	out, st := run(t, `[[ "  " -eq 0 ]] && echo yes`, blameSetup(d))
	if st != 0 || !strings.Contains(out, "yes") {
		t.Errorf("= %q status %d, want a blank operand read as zero", out, st)
	}
}
