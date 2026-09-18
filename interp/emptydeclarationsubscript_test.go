// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration operand whose subscript is **empty** — `typeset 'a[]'=v`,
// which is what `typeset "a[$i]"=v` is once a blank `$i` has gone in, the
// parameters going in before the brackets are read.
//
// It wrote element zero in every dialect and said nothing, so a computed
// subscript that came out blank quietly replaced the array's *first* element at
// status 0 — the worst shape a wrong answer takes, since nothing downstream
// can tell (#3509).
//
// Semantics.EmptyArithSubscript is the axis, read rather than a fourth field:
// every shell that reaches a declaration gives it the disposition it gives the
// same brackets in an expression.
func TestAnEmptySubscriptInADeclarationIsAnsweredLikeAnEmptyOneInAnExpression(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`typeset 'a[]'=v; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	for _, c := range []struct {
		name   string
		p      EmptyArithSubscriptPolicy
		want   string
		says   bool
		status int
	}{
		// The brackets hold an expression that happens to be empty, which is
		// zero, so the operand is element zero and there is nothing to
		// refuse.
		{
			"the empty expression", EmptyArithSubscriptIsTheEmptyExpression,
			"same=0\nnext=0 a=[v 2 3]\n", false, 0,
		},
		// Reported, the element not written, and the rest of the line still
		// running — so the failure is the builtin's and not the script's.
		{
			"reported", EmptyArithSubscriptIsReported,
			"same=1\nnext=0 a=[1 2 3]\n", true, 0,
		},
		// Refused, and a refused declaration store ends the script here.
		{
			"invalid", EmptyArithSubscriptIsInvalid,
			"", true, 1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := emptySubRun(t, c.p, Diagnostics{}, src)
			said := ""
			if i := strings.Index(out, "same="); i >= 0 {
				said, out = out[:i], out[i:]
			} else {
				said, out = out, ""
			}
			if out != c.want {
				t.Errorf("ran %q, want %q", out, c.want)
			}
			if got := said != ""; got != c.says {
				t.Errorf("said %q, want a complaint: %v", said, c.says)
			}
			if st != c.status {
				t.Errorf("status %d, want %d", st, c.status)
			}
		})
	}
}

// The operand carrying **no value** answers the same axis and is worded apart
// by the one column that words them apart: bash refuses the whole operand as a
// name there, builtin and all, where with a value it complains about the
// subscript alone.
func TestAValuelessEmptySubscriptIsAnsweredTheSameWay(t *testing.T) {
	const src = "a=(1 2 3)\n" +
		`typeset 'a[]'; echo "same=$?"` + "\n" +
		`echo "next=$? a=[${a[*]}]"`
	dg := Diagnostics{
		DeclarationEmptySubscript:          "with a value: %[1]s",
		ValuelessDeclarationEmptySubscript: "%[2]s: without one: %[1]s",
	}
	out, _ := emptySubRun(t, EmptyArithSubscriptIsReported, dg, src)
	if !strings.Contains(out, "typeset: without one: a") {
		t.Errorf("out %q does not use the valueless wording, with the builtin in it", out)
	}
	if !strings.Contains(out, "same=1\nnext=0 a=[1 2 3]\n") {
		t.Errorf("out %q does not report and carry on with the array untouched", out)
	}
	// And the column that reads the brackets as the empty expression writes
	// nothing at all here: the operand carries no value, so element zero is
	// declared rather than assigned.
	out, st := emptySubRun(t, EmptyArithSubscriptIsTheEmptyExpression, dg, src)
	if want := "same=0\nnext=0 a=[1 2 3]\n"; out != want || st != 0 {
		t.Errorf("out %q (status %d), want %q at 0", out, st, want)
	}
}

// The wordings are separate fields because one column writes two sentences and
// the other writes one.
func TestAnEmptyDeclarationSubscriptIsWordedPerDialect(t *testing.T) {
	const valued = "a=(1 2 3)\n" + `typeset 'a[]'=v`
	const valueless = "a=(1 2 3)\n" + `typeset 'a[]'`
	two := Diagnostics{
		DeclarationEmptySubscript:          "SUBSCRIPT %[1]s",
		ValuelessDeclarationEmptySubscript: "NAME %[2]s %[1]s",
	}
	out, _ := emptySubRun(t, EmptyArithSubscriptIsReported, two, valued)
	if !strings.Contains(out, "SUBSCRIPT a") || strings.Contains(out, "NAME") {
		t.Errorf("out %q does not use the valued wording", out)
	}
	out, _ = emptySubRun(t, EmptyArithSubscriptIsReported, two, valueless)
	if !strings.Contains(out, "NAME typeset a") || strings.Contains(out, "SUBSCRIPT") {
		t.Errorf("out %q does not use the valueless wording", out)
	}
	// One sentence for both is a dialect's answer rather than a copy, so the
	// same text in both fields has to work.
	one := Diagnostics{
		DeclarationEmptySubscript:          "either way %[1]s",
		ValuelessDeclarationEmptySubscript: "either way %[1]s",
	}
	for _, src := range []string{valued, valueless} {
		out, _ = emptySubRun(t, EmptyArithSubscriptIsReported, one, src)
		if !strings.Contains(out, "either way a") {
			t.Errorf("%q said %q, want the one sentence", src, out)
		}
	}
}

// The attribute refusals answer first, which is measured: zsh's `readonly
// 'a[]'=v` is `can't create readonly array elements` and not the
// empty-subscript sentence.
func TestAnAttributeRefusalBeatsAnEmptyDeclarationSubscript(t *testing.T) {
	out, _ := emptySubRunWith(t, EmptyArithSubscriptIsReported,
		Diagnostics{
			DeclarationEmptySubscript: "EMPTY",
			ReadonlyElementRefusal:    "FROZEN %[1]s[%[2]s]",
		},
		func(s *Semantics) { s.ReadonlyElement = ReadonlyElementRefused },
		"a=(1 2 3)\n"+`readonly 'a[]'=v; echo "same=$?"`)
	if !strings.Contains(out, "FROZEN a[]") {
		t.Errorf("out %q is not the attribute refusal", out)
	}
	if strings.Contains(out, "EMPTY") {
		t.Errorf("out %q reached the empty-subscript refusal first", out)
	}
}

// A subscript that is blank rather than empty is a different axis and keeps
// its own answer: `a[ ]` holds whitespace, which one column reads as the empty
// expression and another refuses, and neither reaches here.
func TestABlankSubscriptIsNotAnEmptyOne(t *testing.T) {
	out, st := emptySubRunWith(t, EmptyArithSubscriptIsReported,
		Diagnostics{DeclarationEmptySubscript: "EMPTY"},
		func(s *Semantics) { s.BlankArithSubscriptIsTheEmptyExpression = Yes },
		"a=(1 2 3)\n"+`typeset 'a[ ]'=v; echo "same=$? a=[${a[*]}]"`)
	if strings.Contains(out, "EMPTY") {
		t.Errorf("out %q read a blank subscript as an empty one", out)
	}
	if want := "same=0 a=[v 2 3]\n"; out != want || st != 0 {
		t.Errorf("out %q (status %d), want %q at 0", out, st, want)
	}
}

// A blank operand subscript reaches the blank axis at all, which it could not
// before: the operand's text was trimmed outright, so `a[ ]` arrived as `a[]`
// and every operand route answered one for the other.
//
// Measured 2026-09-17 with `unset`, the other operand route, where the two
// come apart most plainly in bash 5.3.20: `unset 'a[]'` removes nothing and
// `unset 'a[ ]'` removes element 0. The declaration rows above are the same
// split one builtin over.
func TestABlankOperandSubscriptIsNotTrimmedIntoAnEmptyOne(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		// The blank one reaches the blank axis, which this row answers as
		// the column that reads it: element zero, silently.
		{"blank", `typeset 'a[ ]'=v; echo "same=$? a=[${a[*]}]"`, "same=0 a=[v 2 3]\n"},
		// The empty one reaches the empty axis, which this row answers as
		// the column that reports: nothing written.
		{"empty", `typeset 'a[]'=v; echo "same=$? a=[${a[*]}]"`, "same=1 a=[1 2 3]\n"},
		// And a subscript that is blank on the *outside* still trims to the
		// expression inside it, which is what the trimming was for.
		{"padded", `typeset 'a[ 2 ]'=v; echo "same=$? a=[${a[*]}]"`, "same=0 a=[1 2 v]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := emptySubRunWith(t, EmptyArithSubscriptIsReported,
				Diagnostics{DeclarationEmptySubscript: "EMPTY"},
				func(s *Semantics) { s.BlankArithSubscriptIsTheEmptyExpression = Yes },
				"a=(1 2 3)\n"+c.src)
			if i := strings.Index(out, "same="); i > 0 {
				out = out[i:]
			}
			if out != c.want || st != 0 {
				t.Errorf("out %q (status %d), want %q at 0", out, st, c.want)
			}
		})
	}
}

// An axis nobody answered is refused by name rather than guessed — the value,
// the stream and whether the declaration survives all differ between the
// three, so none of them can stand in for the others.
func TestAnEmptyDeclarationSubscriptRefusesAnUnspecifiedAxis(t *testing.T) {
	out, _ := emptySubRun(t, EmptyArithSubscriptUnspecified, Diagnostics{},
		"a=(1 2 3)\n"+`typeset 'a[]'=v; echo "same=$? a=[${a[*]}]"`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out %q is not a refusal naming the axis", out)
	}
	if !strings.Contains(out, "same=2 a=[1 2 3]") {
		t.Errorf("out %q does not leave the refusal's status behind, array untouched", out)
	}
}

func emptySubRun(t *testing.T, p EmptyArithSubscriptPolicy, dg Diagnostics, src string) (string, int) {
	t.Helper()
	return emptySubRunWith(t, p, dg, nil, src)
}

// emptySubRunWith answers everything a declaration of an element needs except
// the axis under test, so a row varies that one alone.
func emptySubRunWith(t *testing.T, p EmptyArithSubscriptPolicy, dg Diagnostics,
	also func(*Semantics), src string,
) (string, int) {
	t.Helper()
	return optRunAs(t, func(s *Semantics) {
		arraySemantics(s)
		s.TypesetTakesASubscript = Yes
		s.DeclarationTakesASubscript = Yes
		s.SubscriptedOperandTakesALocalDeclaration = Yes
		s.ReadonlyElement = ReadonlyElementWritten
		s.DeclaredNameWithoutValueIsEmpty = No
		s.TypesetLocalNeedsKeywordFunction = No
		s.CompoundElementsGoThroughTheAttribute = Yes
		// The valueless rows go through the column that reads no subscript,
		// so what an operand with a *good* subscript would do is not what
		// they are about.
		s.ValuelessSubscriptedOperand = ValuelessSubscriptedOperandDeclaresTheName
		s.ValuelessDeclarationOfAHeldNameListsIt = No
		// A blank subscript is the neighboring axis and is answered flat.
		s.BlankArithSubscriptIsTheEmptyExpression = No
		// And so is whether a subscript that came out blank is a math error
		// at all — the store reads it one step below this, and every row
		// here is about what the *declaration* does with the brackets.
		s.EmptySubscriptTextIsAMathError = No
		s.EmptyArithSubscript = p
		s.FatalErrorStatusIsOne = Yes
		if also != nil {
			also(s)
		}
	}, dg, src, RouteUnspecified)
}
