// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration inside a function meeting a name the shell has frozen:
// Semantics.DeclarationMayShadowAReadonly. Tests name the axis, never shells.

// shadowingReadonly is declRun's setter for the answer that takes the shadow.
func shadowingReadonly(s *Semantics) {
	s.DeclarationMayShadowAReadonly = Yes
	s.LocalOptions = "aAilprux"
	s.DeclareOptions = "aAilprux"
	// `typeset` declaring a local in a paren-defined function, which is its
	// own axis and not this one.
	s.TypesetLocalNeedsKeywordFunction = No
}

// refusingReadonly is the other answer, with the refusal non-fatal so the
// rest of the function is there to be asked what happened.
func refusingReadonly(s *Semantics) {
	s.DeclarationMayShadowAReadonly = No
	s.LocalOptions = "aAilprux"
	s.DeclareOptions = "aAilprux"
	s.ReadonlyReassignmentFatal = No
	s.ReadonlyReassignmentByDeclarationFatal = No
	s.TypesetLocalNeedsKeywordFunction = No
}

// Where the shadow is taken it is an ordinary local: the body sees its own
// value, the function runs on, and the outer value is back when it returns.
func TestADeclarationMayShadowAFrozenName(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
f() { local x=2; echo "in=[$x]"; echo running; }
f
echo "st=$? out=[$x]"`, shadowingReadonly, Diagnostics{})
	want := "in=[2]\nrunning\nst=0 out=[1]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a local over a frozen name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// And the outer name is frozen **again** afterwards, which is the half a
// shadow that merely cleared the attribute would get wrong: a function that
// thawed a readonly for good is a hole in the whole point of the attribute.
func TestTheOuterNameIsFrozenAgainAfterTheShadow(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
f() { local x=2; }
f
x=9
echo "out=[$x]"`, func(s *Semantics) {
		shadowingReadonly(s)
		s.ReadonlyReassignmentFatal = No
	}, Diagnostics{})
	if out != "out=[1]\n" || st != 0 {
		t.Errorf("an assignment after the shadow returned = %q (status %d), want the "+
			"name still frozen at 1", out, st)
	}
	if !strings.Contains(errs, "readonly") {
		t.Errorf("the assignment said %q, want it refused", errs)
	}
}

// Every spelling of the declaration takes the shadow, which is why this is
// one field and not five.
func TestEverySpellingOfTheDeclarationTakesTheShadow(t *testing.T) {
	for _, tc := range []struct{ decl, want string }{
		{`local x=2`, "in=[2]"},
		{`typeset x=3`, "in=[3]"},
		{`local -r x=4`, "in=[4]"},
		{`local -x x=5`, "in=[5]"},
	} {
		out, errs, st := declRun(t, "typeset -r x=1\nf() { "+tc.decl+
			`; echo "in=[$x]"; }
f
echo "out=[$x]"`, shadowingReadonly, Diagnostics{})
		want := tc.want + "\nout=[1]\n"
		if out != want || st != 0 || errs != "" {
			t.Errorf("%s over a frozen name = %q (stderr %q, status %d), want %q",
				tc.decl, out, errs, st, want)
		}
	}
}

// The valueless form as well, and it is the one that used to shadow in
// silence: with no value there was no assignment, so nothing met the check
// and the local was made whatever the answer was.
func TestAValuelessDeclarationMeetsTheSameQuestion(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
f() { local x; echo "in=[${x-UNSET}]"; }
f`, refusingReadonly, Diagnostics{})
	if out != "in=[1]\n" || st != 0 {
		t.Errorf("a valueless local over a frozen name = %q (status %d), want the "+
			"outer value still in view", out, st)
	}
	if !strings.Contains(errs, "readonly") {
		t.Errorf("it said %q, want the refusal", errs)
	}
}

// Where the shadow is refused, three things follow together: the outer value
// stays in view, the builtin reports 1, and the **function runs on**.
func TestARefusedShadowReportsAndTheFunctionRunsOn(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
f() { local x=2; echo "lst=$?"; echo "in=[$x]"; echo running; }
f
echo "fst=$? out=[$x]"
echo after`, refusingReadonly, Diagnostics{
		ReadonlyVariableInDeclaration: "%[2]s: %[1]s: readonly variable",
		ReadonlyRefusalNamesBuiltin:   map[string]bool{"local": true},
	})
	want := "lst=1\nin=[1]\nrunning\nfst=0 out=[1]\nafter\n"
	if out != want || st != 0 {
		t.Errorf("a refused shadow = %q (status %d), want %q", out, st, want)
	}
	if errs != "testsh: local: x: readonly variable\n" {
		t.Errorf("it said %q, want the builtin named", errs)
	}
}

// And the remaining operands are still declared: only the frozen one is
// refused. A refusal that gave up the whole command would leave `y` and `z`
// global, which is a change to the caller's variables that nothing said.
func TestARefusedOperandDoesNotCostTheOthers(t *testing.T) {
	out, errs, st := declRun(t, `typeset -r x=1
y=outer; z=outer
f() { local y=1 x=5 z=2; echo "in y=[$y] x=[$x] z=[$z]"; }
f
echo "out y=[$y] z=[$z]"`, refusingReadonly, Diagnostics{})
	want := "in y=[1] x=[1] z=[2]\nout y=[outer] z=[outer]\n"
	if out != want || st != 0 {
		t.Errorf("a refused operand among others = %q (status %d), want %q",
			out, st, want)
	}
	if !strings.Contains(errs, "readonly") {
		t.Errorf("it said %q, want the refusal", errs)
	}
}

// The unanswered axis refuses rather than picking a shell, and the
// declaration is not made either way.
func TestTheShadowRefusesWithoutTheAxis(t *testing.T) {
	out, errs, _ := declRun(t, `typeset -r x=1
f() { local x=2; echo "in=[$x]"; }
f`, func(s *Semantics) {
		s.DeclarationMayShadowAReadonly = Unspecified
		s.LocalOptions = "aAilprux"
	}, Diagnostics{})
	want := "testsh: a declaration shadowing a readonly name: the shells disagree here " +
		"and no dialect was chosen\n"
	if errs != want {
		t.Errorf("a shadow with no axis = %q, want %q", errs, want)
	}
	// The declaration is not made — `in=[1]` is the caller's value, not a
	// local of any kind — which is the whole of what an unanswered axis
	// costs here. Reporting it and then declaring anyway is the silent
	// wrong answer; abandoning the function is not what any shell does.
	if out != "in=[1]\n" {
		t.Errorf("it printed %q, want the outer value and no local", out)
	}
}

// Asked only where a declaration meets a frozen name, so an ordinary local
// and an assignment at top level never reach it — which is what keeps a
// dialect that has answered nothing from refusing the common path.
func TestTheQuestionIsAskedOnlyAtTheFrozenName(t *testing.T) {
	out, errs, st := declRun(t, `y=outer
f() { local y=inner; echo "in=[$y]"; }
f
echo "out=[$y]"`, func(s *Semantics) {
		s.DeclarationMayShadowAReadonly = Unspecified
		s.LocalOptions = "aAilprux"
	}, Diagnostics{})
	want := "in=[inner]\nout=[outer]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("an ordinary local = %q (stderr %q, status %d), want %q — the axis "+
			"must not be asked here", out, errs, st, want)
	}
}

// A frozen name assigned at *top level* is the ordinary refusal and not this
// question: there is no scope to shadow into, so the answer changes nothing.
func TestATopLevelAssignmentIsNotThisQuestion(t *testing.T) {
	_, errs, st := declRun(t, `typeset -r x=1
typeset x=2`, func(s *Semantics) {
		s.DeclarationMayShadowAReadonly = Unspecified
		s.DeclareOptions = "aAilprux"
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = No
	}, Diagnostics{})
	if strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("a top-level declaration asked the shadow axis: %q", errs)
	}
	if !strings.Contains(errs, "readonly") || st != 1 {
		t.Errorf("it said %q (status %d), want the ordinary refusal at 1", errs, st)
	}
}
