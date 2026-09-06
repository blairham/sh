// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"

	"github.com/blairham/sh/syntax"
)

// operandDiags is a vector that words the two operand failures apart, so a
// test can tell which one a sentence came from without naming a shell.
func operandDiags() Diagnostics {
	return Diagnostics{
		ArithError:            "%[1]s: %[2]s",
		ArithOperandExpected:  "found <%[1]s>",
		ArithExpressionRanOut: "ran out",
	}
}

// A missing operand is two sentences and not one. The wording used to be the
// end-of-input one whatever had happened, so an expression that found
// something it could not use claimed it had run out — which is what two of the
// panel word apart and what nobody noticed, because the third words both the
// same way.
func TestAMissingOperandIsTwoWordings(t *testing.T) {
	d := operandDiags()
	for _, tc := range []struct {
		src  string
		want string
	}{
		{"1+", "1+: ran out"},
		{"~", "~: ran out"},
		{"%", "%: found <%>"},
		{"1+&2", "1+&2: found <&2>"},
	} {
		err := arithErr(tc.src, syntax.Core())
		if err == nil {
			t.Fatalf("%s: read cleanly, want a failure", tc.src)
		}
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
		}
	}
}

// A dialect that words the two the same way states the sentence once. Leaving
// the second field empty means "the same as the first" rather than "no
// wording", so a preset does not have to repeat itself to say it does not
// distinguish the cases.
func TestOneWordingServesBothWhenTheSecondIsEmpty(t *testing.T) {
	// The sentence is deliberately not the wording a missing field falls back
	// to, so the test tells "the other field was consulted" from "nothing was
	// and the default stood in".
	d := Diagnostics{ArithError: "%[1]s: %[2]s", ArithOperandExpected: "one sentence for both"}
	for _, src := range []string{"1+", "%"} {
		err := arithErr(src, syntax.Core())
		if err == nil {
			t.Fatalf("%s: read cleanly, want a failure", src)
		}
		if got, want := d.ParseFailure(err), src+": one sentence for both"; got != want {
			t.Errorf("%s: got %q, want %q", src, got, want)
		}
	}
}

// The dialect that blames a substring's offset together with the rest of the
// range is the dialect that *reads* the rest of the range, so what ran out for
// this parser did not run out for it. Naming the whole range and reading the
// whole range are one fact about that shell, which is why the blamed text is
// enough to know it and no second flag has to be kept in step with the first.
func TestBlamingPastTheExpressionMakesARanOutFailureAFoundOne(t *testing.T) {
	d := operandDiags()
	d.SubstringErrorNamesTheWholeRange = true
	src := `x=abcdef; echo "${x:1+:2}"`
	out, _ := runGrammar(t, src, nil, func(r *Runner) { r.Diagnostics = &d })
	if !strings.Contains(out, "1+:2: found <+>") {
		t.Errorf("output = %q, want the whole range blamed with the found wording", out)
	}
	// A length has nothing written after it, so it is the plain case even in
	// that dialect and keeps the ran-out wording.
	out, _ = runGrammar(t, `x=abcdef; echo "${x:2:1+}"`, nil, func(r *Runner) { r.Diagnostics = &d })
	if !strings.Contains(out, "1+: ran out") {
		t.Errorf("output = %q, want a length blamed alone with the ran-out wording", out)
	}
}

// Without that answer the range is blamed as the parser saw it, and the
// failure stays the one the parser found.
func TestBlamingOnlyTheOffsetKeepsTheRanOutWording(t *testing.T) {
	d := operandDiags()
	out, _ := runGrammar(t, `x=abcdef; echo "${x:1+:2}"`, nil, func(r *Runner) { r.Diagnostics = &d })
	if !strings.Contains(out, "1+: ran out") {
		t.Errorf("output = %q, want the offset alone with the ran-out wording", out)
	}
}

// arithErr reads one expression as this dialect and returns what the read had
// to say — the route the interpreter itself takes when the command runs,
// which is the only place a bad expression is refused now (#865).
func arithErr(expr string, d syntax.Dialect) error {
	p := syntax.NewParser("", d)
	p.ParseArithFor(expr, syntax.Pos{})
	return p.Err()
}
