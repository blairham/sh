// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The `-g` letter written on `local` rather than on the declaration word.
//
// It is not a second question: Semantics.LocalOptions says whether the word
// takes the letter and Semantics.DeclareGlobalReachesPastALocal says where the
// letter writes, and those two together are the whole of it. What the letter
// means is that the line declares **no local at all**, so every step `local`
// adds over the declaration word — the shadow, the scope's saved export, the
// freeze that comes off at the return — is about a binding this line does not
// make, and the declaration is the one route.
//
// Named for the axes and never for a shell, as everything in this package is.

// localGlobalLetter is the vector these rows share: a `local` that takes the
// letter, under each answer to where the letter writes.
func localGlobalLetter(reaches Answer) func(*Semantics) {
	return func(s *Semantics) {
		globalLetter(reaches)(s)
		s.LocalOptions = "aAgiprx"
	}
}

// TestTheGlobalLetterOnTheLocalWordTakesNoLocal: the letter reaches the word,
// and the axis decides the same thing under it that it decides under the
// declaration word.
func TestTheGlobalLetterOnTheLocalWordTakesNoLocal(t *testing.T) {
	const src = "x=out\nf() { local x=in; local -g x=new; echo in=$x; }\nf\necho out=$x"

	out, errs, st := declRun(t, src, localGlobalLetter(Yes), Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("stderr %q status %d, want a clean run", errs, st)
	}
	if !strings.Contains(out, "in=in") || !strings.Contains(out, "out=new") {
		t.Errorf("stdout = %q, want the local untouched and the shell's own cell written", out)
	}

	out, errs, st = declRun(t, src, localGlobalLetter(No), Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("stderr %q status %d, want a clean run", errs, st)
	}
	if !strings.Contains(out, "in=new") || !strings.Contains(out, "out=out") {
		t.Errorf("stdout = %q, want the visible cell assigned and the outer one left alone", out)
	}
}

// TestTheGlobalLetterOnTheLocalWordSurvivesTheReturn is the control that says
// the rows above are about the letter rather than about a local that happened
// to be written over: with nothing standing on the name the axis is not asked
// at all, and what the letter declares has to outlive the call.
func TestTheGlobalLetterOnTheLocalWordSurvivesTheReturn(t *testing.T) {
	out, errs, st := declRun(t, "f() { local -g g1=set; }\nf\necho g=$g1",
		localGlobalLetter(Yes), Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("stderr %q status %d, want a clean run", errs, st)
	}
	if !strings.Contains(out, "g=set") {
		t.Errorf("stdout = %q, want the declaration to outlive the call", out)
	}
	// And the same line without the letter, which is what makes the row
	// above a measurement of the letter: a plain `local` is gone at the
	// return.
	out, _, _ = declRun(t, "f() { local g2=set; }\nf\necho g=[${g2-U}]",
		localGlobalLetter(Yes), Diagnostics{})
	if !strings.Contains(out, "g=[U]") {
		t.Errorf("stdout = %q, want a plain local gone at the return", out)
	}
}

// TestTheGlobalLetterOnTheLocalWordReachesTheLiteralOperand: an array literal
// is assigned after the builtin has returned, so the letter has to reach the
// operand's own store and not only the scalar one. The route the declaration
// word needed interp/globaloperand.go for, asked under this word.
func TestTheGlobalLetterOnTheLocalWordReachesTheLiteralOperand(t *testing.T) {
	const src = "q=(9 9)\nf() { local q=(5); local -ga q=(1 2); echo in=${q[*]}; }\nf\necho out=${q[*]}"
	out, errs, st := declRun(t, src, localGlobalLetter(Yes), Diagnostics{})
	if errs != "" || st != 0 {
		t.Fatalf("stderr %q status %d, want a clean run", errs, st)
	}
	if !strings.Contains(out, "in=5") || !strings.Contains(out, "out=1 2") {
		t.Errorf("stdout = %q, want the local array untouched and the shell's own written", out)
	}
}

// TestAWordWithoutTheGlobalLetterRefusesIt is the other control, and it is the
// one that says LocalOptions is what decides: a dialect whose `local` has no
// such letter must refuse the option rather than silently declare a global.
func TestAWordWithoutTheGlobalLetterRefusesIt(t *testing.T) {
	const src = "x=out\nf() { local -g x=new; }\nf\necho st=$?\necho out=$x"
	out, errs, _ := declRun(t, src, func(s *Semantics) {
		globalLetter(Yes)(s)
		s.LocalOptions = "aAiprx"
	}, Diagnostics{})
	if errs == "" || strings.Contains(out, "st=0") {
		t.Errorf("stdout %q stderr %q, want the letter refused", out, errs)
	}
	if !strings.Contains(out, "out=out") {
		t.Errorf("stdout = %q, want nothing written", out)
	}
}

// TestTheGlobalLetterIsStillLocalsOwnWordOutsideAFunction: the letter says
// which cell is written, and says nothing about where the word may be
// written. The refusal belongs to `local` and stands in front of the
// declaration — which the declaration word has no equivalent of, so a
// delegation reached ahead of it would make `local -g x=1` legal at the top
// level.
func TestTheGlobalLetterIsStillLocalsOwnWordOutsideAFunction(t *testing.T) {
	out, errs, st := declRun(t, "local -g x=new\necho out=[${x-U}]", func(s *Semantics) {
		localGlobalLetter(Yes)(s)
		s.LocalOutsideAFunctionIsAnError = Yes
		s.LocalOutsideAFunctionIsFatal = No
	}, Diagnostics{})
	if st != 0 || errs == "" {
		t.Fatalf("status %d stderr %q, want the refusal reported", st, errs)
	}
	if !strings.Contains(out, "out=[U]") {
		t.Errorf("stdout = %q, want the refused declaration to have written nothing", out)
	}
}
