// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// Where `set -x` writes a declaration utility's **array literal** operand, and
// how the list is spelled there — Diagnostics.TraceDeclarationArrayOperand and
// TraceArrayOperandQuotesEveryElement, and #3567, where the operand was
// written nowhere at all and the command word stood alone.
//
// Named for the fields and not for the shells; which value each preset holds
// is asserted in dialect/xtracearrayoperand_test.go against the panel's own
// bytes. What is asserted here is that the fields are different answers, that
// each can be reached, and — the part that makes the suite worth running —
// that neither reaches a word it must not.
//
// The probe carries three things on purpose, and every one of them
// discriminates:
//
//   - `b=("$x" r)` **as an operand**, which is what moves. Its value is a
//     quoted parameter holding a space, so a line built from the words rather
//     than from the expansion says something different.
//   - `y=1` **on the same command**, which is a *scalar* operand and must not
//     move: Diagnostics.TraceDeclarationOperand is the field that governs it,
//     and one column answers the two differently. Without this row a single
//     field would pass every assertion below.
//   - `c=("$x" r)` **as a bare assignment**, which must not move either and
//     must keep the words as the script wrote them. It is what says the
//     question is about the operand position rather than about array
//     literals: permissive() answers
//     Semantics.TraceArrayLiteralShowsTheExpandedElements `No`, so the two
//     lines are built from opposite readings in one run.
const arrayOperandProbe = "set -x\nx='p q'\ntypeset y=1 b=(\"$x\" r)\nc=(\"$x\" r)\n"

func arrayOperandTrace(t *testing.T, d Diagnostics) string {
	t.Helper()
	d.TraceQuoting = QuoteShell
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	return traceOf(t, arrayOperandProbe, sem, d)
}

// TestAnArrayOperandStandsOnTheCommandLine is the zero value: the literal is
// written where the script wrote it, from what its elements expanded to.
func TestAnArrayOperandStandsOnTheCommandLine(t *testing.T) {
	wantTrace(t, arrayOperandTrace(t, Diagnostics{}),
		"+ x='p q'\n+ typeset y=1 b=('p q' r)\n+ c=(\"$x\" r)\n")
}

// TestAnArrayOperandIsSplitInFrontOfTheCommand takes it off the command line
// and writes it as the assignment it performs, leaving the bare name.
//
// The scalar operand beside it does not move, which is the whole reason this
// is a second field.
func TestAnArrayOperandIsSplitInFrontOfTheCommand(t *testing.T) {
	wantTrace(t, arrayOperandTrace(t, Diagnostics{
		TraceDeclarationArrayOperand: TraceOperandSplitBefore,
	}), "+ x='p q'\n+ b=('p q' r)\n+ typeset y=1 b\n+ c=(\"$x\" r)\n")
}

// TestTheTwoOperandFieldsMoveSeparately is the control the pair needs: the
// scalar field moves `y=1` and leaves the literal where it is, which is the
// mirror image of the test above over the same probe.
func TestTheTwoOperandFieldsMoveSeparately(t *testing.T) {
	wantTrace(t, arrayOperandTrace(t, Diagnostics{
		TraceDeclarationOperand: TraceOperandSplitBefore,
	}), "+ x='p q'\n+ y=1\n+ typeset y b=('p q' r)\n+ c=(\"$x\" r)\n")
}

// TestAnArrayOperandQuotesEveryElement is the second field: each element is
// single-quoted whether or not the ordinary rule would reach for quotes.
//
// `r` is the row that discriminates — it needs no quoting under any value of
// TraceQuoting — and the bare `c=(…)` on the same run is untouched, so the
// field reaches the operand's list and nothing else.
func TestAnArrayOperandQuotesEveryElement(t *testing.T) {
	wantTrace(t, arrayOperandTrace(t, Diagnostics{
		TraceArrayOperandQuotesEveryElement: true,
	}), "+ x='p q'\n+ typeset y=1 b=('p q' 'r')\n+ c=(\"$x\" r)\n")
}

// TestAnArrayOperandIsExpandedOnceAndBeforeTheCommandLine: the elements are
// expanded ahead of the command's own line — which is what makes an assignment
// line in front of it possible at all — and exactly once, side effects
// included.
//
// A command substitution is the only element that can say both: its own trace
// stands above the assignment line and above the command line, and it appears
// once rather than twice. Expanding a second time to print it is the double
// run #1915 fixed for a scalar's value.
func TestAnArrayOperandIsExpandedOnceAndBeforeTheCommandLine(t *testing.T) {
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	got := traceOf(t, "set -x\ntypeset b=($(echo z))\n", sem, Diagnostics{
		TraceQuoting:                 QuoteShell,
		TraceDeclarationArrayOperand: TraceOperandSplitBefore,
	})
	wantTrace(t, got, "+ echo z\n+ b=(z)\n+ typeset b\n")
}

// TestAnEmptyArrayOperandIsAnEmptyPair: nothing between the parentheses is
// still an assignment, and the line says so rather than being dropped.
//
// The dialect that reads the empty pair as a compound body is the one that
// writes no line for it, and it is asked in dialect/ where the grammar that
// has the construct lives.
func TestAnEmptyArrayOperandIsAnEmptyPair(t *testing.T) {
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	got := traceOf(t, "set -x\ntypeset b=()\n", sem, Diagnostics{
		TraceQuoting:                 QuoteShell,
		TraceDeclarationArrayOperand: TraceOperandSplitBefore,
	})
	wantTrace(t, got, "+ b=()\n+ typeset b\n")
}

// TestASubscriptedArrayOperandIsWrittenFromItsHalves: a `[sub]=value` element
// of an operand's literal is written from the two things it expanded to, and
// each half is quoted by the same rule the bare elements are.
//
// `[$k]` is the discriminator: an element written back as the script typed it
// would say `[$k]`, and what the shell is about to store is `[kk]`. The
// subscript is the *text* it expanded to and never a number it might evaluate
// to, which `[i+1]` is here to say.
func TestASubscriptedArrayOperandIsWrittenFromItsHalves(t *testing.T) {
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	sem.TraceSubscriptedArrayLiteralIsElementAssignments = No
	// What the *store* then makes of `[i+1]` is a question of its own, and it
	// is answered here only so it is not asked: the line above is written
	// before the store runs and says the same thing either way.
	sem.ArrayLiteralSubscriptIsAKey = Yes
	const src = "k=kk\nset -x\ntypeset m=([$k]=v [i+1]=w)\n"
	d := Diagnostics{
		TraceQuoting:                 QuoteShell,
		TraceDeclarationArrayOperand: TraceOperandSplitBefore,
	}
	wantTrace(t, traceOf(t, src, sem, d), "+ m=([kk]=v [i+1]=w)\n+ typeset m\n")
	d.TraceArrayOperandQuotesEveryElement = true
	wantTrace(t, traceOf(t, src, sem, d), "+ m=(['kk']='v' ['i+1']='w')\n+ typeset m\n")
	// And the column that spells such a literal as the element writes it
	// performs spells the *operand* the same way, one line each — which is
	// what says the operand borrows that axis rather than restating it.
	sem.TraceSubscriptedArrayLiteralIsElementAssignments = Yes
	wantTrace(t, traceOf(t, src, sem, Diagnostics{
		TraceQuoting:                 QuoteShell,
		TraceDeclarationArrayOperand: TraceOperandSplitBefore,
	}), "+ m[kk]=v\n+ m[i+1]=w\n+ typeset m\n")
}

// TestAnArrayOperandsValueLandsWhateverTheTraceDid is the assertion the whole
// file rests on: moving the elements' expansion ahead of the utility must not
// change what the name comes to hold, and must not change it when nothing is
// tracing either.
//
// Run three ways over one value, because the expansion takes a different route
// in each: untraced, traced with the literal left on the command line, and
// traced with it split in front.
func TestAnArrayOperandsValueLandsWhateverTheTraceDid(t *testing.T) {
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	const tail = "set +x\nprintf '[%s]' \"${b[0]}\" \"${b[1]}\"\n"
	for _, c := range []struct {
		name  string
		trace string
		d     Diagnostics
	}{
		{"untraced", "", Diagnostics{}},
		{"traced, on the line", "set -x\n", Diagnostics{}},
		{
			"traced, split in front", "set -x\n",
			Diagnostics{TraceDeclarationArrayOperand: TraceOperandSplitBefore},
		},
	} {
		d := c.d
		d.TraceQuoting = QuoteShell
		src := "x='p q'\n" + c.trace + "typeset b=(\"$x\" r)\n" + tail
		if got := outputOf(t, src, sem, d); got != "[p q][r]" {
			t.Errorf("%s: stored %q, want %q", c.name, got, "[p q][r]")
		}
	}
}

// outputOf is traceOf's other half: what the script wrote to its standard
// output, with the trace kept off it.
func outputOf(t *testing.T, src string, sem Semantics, diag Diagnostics) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errOut bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errOut,
		Semantics: &sem, Diagnostics: &diag,
		Name: "sh", Env: testPATH(),
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
