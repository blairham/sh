// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// What a declaration utility is handed for an **appending** array-literal
// operand — Semantics.DeclarationTakesAnAppendingArrayOperand, and #3805,
// where every dialect handed over the bare name and the operator was lost
// between the parser and the builtin.
//
// Named for the axis and not for a shell; which value each preset holds, and
// what it then writes, is in dialect/appendarrayoperand_test.go against the
// panel's own bytes.
//
// What is asserted is the **name that was declared**, and never the wording of
// a complaint. The answer that takes the operator leaves the elements on `u`;
// the answer that keeps it leaves nothing on `u` at all, because the utility
// was handed a name it will not take. A suite written against the sentence
// would pass for a shell that printed it and stored the array anyway.

// appendArrayOperandRun runs a script with one axis moved and returns both
// streams joined, which is how a refusal and a listing are compared together.
func appendArrayOperandRun(t *testing.T, takes Answer, src string) string {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	sem.DeclarationCommandWord = DeclarationByUnquotedLiteralWord
	sem.DeclarationTakesAnAppendingArrayOperand = takes
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &out,
		Semantics: &sem, Name: "sh", Env: testPATH(),
	})
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

// TestAnAppendingArrayOperandKeepsItsMarker is the axis: `typeset u+=(3 4)`
// reaches the utility as `u` where the operator is taken and as `u+` where it
// is not.
//
// The name the utility declared is what is asserted, not the complaint: the
// answer that keeps the marker declares nothing called `u`, and the answer
// that takes it declares `u` and stores the elements.
func TestAnAppendingArrayOperandKeepsItsMarker(t *testing.T) {
	const src = "typeset u+=(3 4)\n" + `printf "[%s]" "${u[@]}"` + "\n"

	took := appendArrayOperandRun(t, Yes, src)
	if !strings.HasSuffix(took, "[3][4]") {
		t.Errorf("taken: %q, want it to end in the two elements", took)
	}
	if strings.Contains(took, "u+") {
		t.Errorf("taken: %q names u+, so the marker reached the utility", took)
	}

	kept := appendArrayOperandRun(t, No, src)
	if !strings.Contains(kept, "u+") {
		t.Errorf("kept: %q, want the utility to have been handed u+", kept)
	}
	if strings.Contains(kept, "[3]") {
		t.Errorf("kept: %q stored the elements, and the declaration was refused", kept)
	}
}

// TestAPlainArrayOperandIsTheSameWordUnderBothAnswers is the control that
// makes the test above mean something: an operand written *without* the
// operator has no marker to place, so neither answer may move it. Without this
// a change that put a `+` on every operand would pass every assertion above.
func TestAPlainArrayOperandIsTheSameWordUnderBothAnswers(t *testing.T) {
	const src = "typeset a=(3 4)\n" + `printf "[%s]" "${a[@]}"` + "\n"
	took := appendArrayOperandRun(t, Yes, src)
	kept := appendArrayOperandRun(t, No, src)
	if took != kept {
		t.Errorf("a plain literal moved with the axis:\ntaken %q\nkept  %q", took, kept)
	}
	if took != "[3][4]" {
		t.Errorf("a plain literal = %q, want %q", took, "[3][4]")
	}
}

// TestAnAppendingScalarOperandIsNotThisAxis is the second control, and it is
// the reason there are two fields rather than one. A *scalar* append operand
// is a word the utility reads for itself — DeclarationTakesAnAppendOperand is
// what decides it — so moving this axis must not touch it.
func TestAnAppendingScalarOperandIsNotThisAxis(t *testing.T) {
	const src = "u=1\ntypeset u+=3\n" + `printf "[%s]" "$u"` + "\n"
	took := appendArrayOperandRun(t, Yes, src)
	kept := appendArrayOperandRun(t, No, src)
	if took != kept {
		t.Errorf("a scalar operand moved with the array axis:\ntaken %q\nkept  %q", took, kept)
	}
}

// TestAnAppendingArrayOperandJoinsWhatTheNameHeld is the value row under the
// answer that takes the operator: the elements go after the ones the name
// already had rather than replacing them, which is what says the word handed
// over was still read as an append.
func TestAnAppendingArrayOperandJoinsWhatTheNameHeld(t *testing.T) {
	const src = "typeset a=(1 2)\ntypeset a+=(3 4)\n" + `printf "[%s]" "${a[@]}"` + "\n"
	took := appendArrayOperandRun(t, Yes, src)
	if took != "[1][2][3][4]" {
		t.Errorf("joined = %q, want %q", took, "[1][2][3][4]")
	}
}
