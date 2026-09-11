// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration's plain word landing on a cell that is really holding an
// array — see Semantics.ScalarOverACompoundIsAnInconsistentType and #1666.
//
// Named for the axis and never for a shell.

func inconsistentRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.TypesetLocalNeedsKeywordFunction = No
		// What a scalar store does to a compound where the line is taken,
		// which is the other side's answer and a different axis.
		s.ScalarAssignedOverACompoundReplacesTheName = Yes
		s.ScalarOverACompoundIsAnInconsistentType = a
	}, Diagnostics{})
}

// The headline, both ways. Under Yes the script ends there, which is the half
// a status alone would not show — `tail` never runs.
func TestAScalarDeclaredOverACompoundIsAnAxis(t *testing.T) {
	const src = `b=(x y); typeset b=q; echo "st=$? [${b[*]}]"; echo tail`
	out, errs, st := inconsistentRun(t, src, Yes)
	if out != "" || st == 0 {
		t.Errorf("yes: got %q/%d, want the script ended with nothing written", out, st)
	}
	if !strings.Contains(errs, "b: inconsistent type for assignment") {
		t.Errorf("yes: stderr %q, want the name and the complaint", errs)
	}
	out, errs, st = inconsistentRun(t, src, No)
	if want := "st=0 [q]\ntail\n"; out != want || st != 0 || errs != "" {
		t.Errorf("no: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The keyed half is the same refusal, so it is one question and not two.
func TestAScalarDeclaredOverATableIsTheSameRefusal(t *testing.T) {
	out, errs, st := inconsistentRun(t, `typeset -A m; m[k]=v; typeset m=q; echo tail`, Yes)
	if out != "" || st == 0 || !strings.Contains(errs, "m: inconsistent type for assignment") {
		t.Errorf("got %q/%d stderr %q, want the table refused too", out, st, errs)
	}
}

// It is the **declaration** that refuses and not the store: the same value
// through a bare assignment is taken in both shells, which is what says this
// is not ScalarAssignedOverACompoundReplacesTheName seen from another angle.
func TestAPlainAssignmentOverACompoundIsNotRefused(t *testing.T) {
	out, errs, st := inconsistentRun(t, `b=(x y); b=q; echo "[$b]"; echo tail`, Yes)
	if want := "[q]\ntail\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// Nor is it the fresh cell: a declaration inside a function writes a cell it
// has just made, which holds nothing whatever the caller left.
func TestAFreshLocalOverACallersCompoundIsTaken(t *testing.T) {
	out, errs, st := inconsistentRun(t,
		`b=(x y); f(){ local b=q; echo "in=[$b]"; }; f; printf 'out=[%s]\n' "${b[*]}"`, Yes)
	if want := "in=[q]\nout=[x y]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The refusal has a **direction**, and the asymmetry is the thing to pin: the
// mirror image — an array letter over a standing scalar — is taken in silence.
func TestAnArrayDeclarationOverAScalarIsNotRefused(t *testing.T) {
	out, errs, st := declRun(t, `b=1; typeset -a b; echo "st=$?"; echo tail`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ScalarOverACompoundIsAnInconsistentType = Yes
		s.ScalarUnderAnArrayDeclaration = ScalarUnderACompoundDiscardsIt
	}, Diagnostics{})
	if want := "st=0\ntail\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The other declaration words refuse it in their own name, which is what says
// the rule belongs to the declaration utilities rather than to one word. The
// builtin reaches the diagnostic through the location, so each line is
// checked for its own name there.
func TestTheOtherDeclarationWordsRefuseItToo(t *testing.T) {
	for _, src := range []string{
		`b=(x y); readonly b=q; echo tail`,
		`b=(x y); export b=q; echo tail`,
	} {
		out, errs, st := inconsistentRun(t, src, Yes)
		if out != "" || st == 0 || !strings.Contains(errs, "b: inconsistent type for assignment") {
			t.Errorf("%s: got %q/%d stderr %q, want the refusal", src, out, st, errs)
		}
	}
}

// A name holding nothing raises no question at all, which is the control the
// whole rule turns on.
func TestAScalarDeclaredOverNothingIsNotRefused(t *testing.T) {
	out, errs, st := inconsistentRun(t, `typeset b=q; echo "[$b]"`, Yes)
	if out != "[q]\n" || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want the declaration taken", out, st, errs)
	}
}

// The dialect's own wording reaches the sentence, with the name in it.
func TestTheInconsistentTypeWordingIsTheDialects(t *testing.T) {
	_, errs, _ := declRun(t, `b=(x y); typeset b=q`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ScalarOverACompoundIsAnInconsistentType = Yes
	}, Diagnostics{InconsistentType: "%s is two things at once"})
	if !strings.Contains(errs, "b is two things at once") {
		t.Errorf("stderr %q, want the dialect's wording", errs)
	}
}
