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

// An assignment prefix in front of a builtin is two things at once — the
// environment the builtin is handed, or a value the shell holds for one
// command — and the two readings part in two observable places. These pin both
// axes in every one of their positions, and the unanswered position too.
//
// Nothing here names a shell: the dialect packages hold which preset gives
// which value. See interp/prefixpromote.go (#3437).

func builtinPrefixRun(t *testing.T, src string, sem Semantics) (string, string) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem,
		Dir: t.TempDir(), Name: "testsh",
	})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v\nstderr: %s", src, rerr, errs.String())
	}
	return out.String(), errs.String()
}

// builtinPrefixSem answers everything the rows below need except the axis under
// test, so a row reports the axis and not a refusal of something beside it.
func builtinPrefixSem() Semantics {
	s := testSemantics()
	s.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnspecified
	s.DeclarationPromotesThePrefixEntry = Unspecified
	return s
}

// The export attribute moves three ways, and the middle answer is what makes
// this an enum: a name that was already exported separates "left alone" from
// "taken off", and a name that was not separates "left alone" from "given the
// attribute". Neither reading alone can tell all three apart.
func TestThePrefixExportAtABuiltinHasThreePositions(t *testing.T) {
	// `export -p` is the reading, because the shell can answer it itself: a
	// child's environment would need a program, and this package starts none.
	// Two runs rather than two listings in one, so each row reads a listing
	// taken under exactly one prefix.
	for _, c := range []struct {
		name         string
		answer       PrefixExportAtABuiltinPolicy
		already, not bool
	}{
		{"on", PrefixExportAtABuiltinOn, true, true},
		{"unchanged", PrefixExportAtABuiltinUnchanged, true, false},
		{"off", PrefixExportAtABuiltinOff, false, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := builtinPrefixSem()
			sem.PrefixExportAtABuiltin = c.answer
			sem.DeclarationPromotesThePrefixEntry = No
			out, errs := builtinPrefixRun(t, "export z=1\nz=2 export -p", sem)
			if got := strings.Contains(out, `export z="2"`); got != c.already {
				t.Errorf("%v: an already-exported name listed %v, want %v\nout %q err %q",
					c.answer, got, c.already, out, errs)
			}
			out, errs = builtinPrefixRun(t, "c=1\nc=2 export -p", sem)
			if got := strings.Contains(out, `export c="2"`); got != c.not {
				t.Errorf("%v: an unexported name listed %v, want %v\nout %q err %q",
					c.answer, got, c.not, out, errs)
			}
		})
	}
}

// And the attribute is the command's: whatever the answer moved, the name gets
// its own back when the builtin ends.
func TestThePrefixExportAtABuiltinIsGivenBack(t *testing.T) {
	for _, answer := range []PrefixExportAtABuiltinPolicy{
		PrefixExportAtABuiltinOn, PrefixExportAtABuiltinUnchanged, PrefixExportAtABuiltinOff,
	} {
		sem := builtinPrefixSem()
		sem.PrefixExportAtABuiltin = answer
		sem.DeclarationPromotesThePrefixEntry = No
		out, errs := builtinPrefixRun(t, `export z=1
c=1
z=2 true
c=2 true
export -p`, sem)
		if !strings.Contains(out, `export z="1"`) || strings.Contains(out, "export c=") {
			t.Errorf("%v: the listing after the commands was %q, want `z` exported at 1 "+
				"and `c` not exported at all\nerr %q", answer, out, errs)
		}
	}
}

// Unanswered is refused by name rather than guessed at, and the refusal is not
// a silent fallback to one of the three.
func TestAnUnansweredPrefixExportAtABuiltinIsRefused(t *testing.T) {
	sem := builtinPrefixSem()
	sem.DeclarationPromotesThePrefixEntry = No
	_, errs := builtinPrefixRun(t, "c=1\nc=2 true", sem)
	if !strings.Contains(errs, "an assignment prefix moving the export attribute at a builtin") ||
		!strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr = %q, want the axis named in the refusal", errs)
	}
}

// A declaration naming the export or the readonly attribute over the name its
// own prefix is holding either keeps the prefix's value for the shell or gives
// it back, and the two readings leave different values *and* different
// attributes behind.
func TestADeclarationKeepingItsOwnPrefixIsAnAxisWithTwoSides(t *testing.T) {
	src := `b=7
b=8 readonly b
echo "[$b]"
b=9
echo "after [$b]"`
	for _, c := range []struct {
		answer Answer
		want   string
	}{
		{Yes, "[8]\nafter [8]\n"},
		{No, "[7]\nafter [9]\n"},
	} {
		sem := builtinPrefixSem()
		sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
		sem.DeclarationPromotesThePrefixEntry = c.answer
		sem.PrefixRefusalFatality = PrefixRefusalNeverFatal
		sem.PrefixRefusalCostsTheCommand = No
		sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
		out, errs := builtinPrefixRun(t, src, sem)
		if !strings.HasPrefix(out, c.want) {
			t.Errorf("with the axis at %v the declaration left %q, want it to start %q\nerr %q",
				c.answer, out, c.want, errs)
		}
	}
}

// It is the *attribute* that keeps and not the word: a declaration that names
// no attribute at all keeps nothing under either answer, so the axis is not a
// second spelling of "a declaration builtin persists".
func TestADeclarationNamingNoAttributeKeepsNothingEitherWay(t *testing.T) {
	for _, answer := range []Answer{Yes, No} {
		sem := builtinPrefixSem()
		sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
		sem.DeclarationPromotesThePrefixEntry = answer
		sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
		out, errs := builtinPrefixRun(t, "x=1\nx=2 typeset x\necho \"[$x]\"", sem)
		if out != "[1]\n" {
			t.Errorf("with the axis at %v a bare declaration left %q, want \"[1]\\n\"\nerr %q",
				answer, out, errs)
		}
	}
}

// Unanswered is refused by name here too, and only where a prefix is really
// standing in front of the name being declared — an ordinary `readonly` on its
// own line asks nothing.
func TestAnUnansweredDeclarationKeepingIsRefusedOnlyOverItsOwnPrefix(t *testing.T) {
	sem := builtinPrefixSem()
	sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
	if _, errs := builtinPrefixRun(t, "b=7\nreadonly b\nx=1 true", sem); errs != "" {
		t.Errorf("a declaration with no prefix over its name wrote %q, want nothing", errs)
	}
	_, errs := builtinPrefixRun(t, "b=7\nb=8 readonly b", sem)
	if !strings.Contains(errs, "a declaration keeping the value its own assignment prefix set") ||
		!strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("stderr = %q, want the axis named in the refusal", errs)
	}
}

// A declaration that takes a *fresh scope* takes the prefix's entry into it
// instead, so the cell holds the prefix's value and the outer name is given
// back what it had before the command. Core rather than an axis: the three
// columns that hide an outer value at all agree, and the other two never reach
// the question.
func TestAFreshDeclarationOverAPrefixKeepsTheValueInItsOwnCell(t *testing.T) {
	sem := builtinPrefixSem()
	sem.PrefixExportAtABuiltin = PrefixExportAtABuiltinUnchanged
	sem.DeclarationPromotesThePrefixEntry = Yes
	sem.AssignmentPrefixPersistsOnSpecialBuiltin = No
	// The declaration has to take a scope for there to be a fresh cell at
	// all, and the prefix to the call has to be answered — neither is this
	// question, and both are asked by the rows below on the way to it.
	sem.ReadonlyDeclaresALocal = Yes
	sem.PrefixToAFunctionIsExported = No
	sem.AssignmentPrefixPersistsAfterAFunction = No
	// And the hiding has to be live, or the rows below pass for a shell that
	// never hides anything and the probe cannot tell the two readings apart.
	// `local [2]` under DeclaredNameWithoutValueIsEmpty = No and
	// ValuelessDeclarationHidesTheOuterValue = Yes is UNSET without this
	// file's change; with the axes left at the standard's silence it is `2`
	// either way, which is what the first version of this test asserted.
	sem.DeclaredNameWithoutValueIsEmpty = No
	sem.ValuelessDeclarationHidesTheOuterValue = Yes
	out, errs := builtinPrefixRun(t, `b=8
f() { b=4 readonly b; echo "in [$b]"; }
f
echo "out [$b]"
b=9
echo "writable [$b]"
g() { local c; echo "local [${c-UNSET}]"; }
c=2 g
echo "gone [${c-UNSET}]"`, sem)
	want := "in [4]\nout [8]\nwritable [9]\nlocal [2]\ngone [UNSET]\n"
	if out != want {
		t.Errorf("a fresh declaration over a prefix = %q, want %q\nerr %q", out, want, errs)
	}
}
