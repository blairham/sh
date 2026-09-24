// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration of a name a *calling* function froze — see
// Semantics.DeclarationMayShadowAnEnclosingScopesReadonly, which carries the
// measurement. Tests name the axis, never shells.
//
// Asked only behind a `no` to DeclarationMayShadowAReadonly, so every case
// here sets that first: where the wider answer is yes there is no refusal
// for this one to narrow.

func enclosingFreeze(a Answer) func(*Semantics) {
	return func(s *Semantics) {
		s.DeclarationMayShadowAReadonly = No
		s.DeclarationMayShadowAnEnclosingScopesReadonly = a
		s.LocalOptions = "aAilprux"
		s.DeclareOptions = "aAilprux"
		s.TypesetLocalNeedsKeywordFunction = No
		// The refusal is non-fatal in all three of its spellings, so the
		// rest of the function is there to be asked what happened. Which
		// refusals end a script is its own family of axes and not this one
		// — see readonlyshadow_test.go, whose setter this follows.
		s.ReadonlyReassignmentFatal = No
		s.ReadonlyReassignmentByDeclarationFatal = No
		s.ReadonlyReassignmentBySpecialBuiltinFatal = No
	}
}

// TestADeclarationMayShadowACallersFrozenLocal is the answer that reads the
// freeze as the caller's: the deeper declaration is an ordinary local, and
// the caller's frozen value is untouched when the call returns.
func TestADeclarationMayShadowACallersFrozenLocal(t *testing.T) {
	const src = `inner() { local x=I; echo "inner=[$x]"; }
outer() { local -r x=O; inner; echo "outer=[$x]"; }
outer`
	out, errs, st := declRun(t, src, enclosingFreeze(Yes), Diagnostics{})
	want := "inner=[I]\nouter=[O]\n"
	if out != want || errs != "" || st != 0 {
		t.Errorf("shadowing a caller's freeze = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// TestADeclarationIsRefusedOverACallersFrozenLocal is the other answer: the
// freeze reaches every depth, so the callee reads the caller's value.
func TestADeclarationIsRefusedOverACallersFrozenLocal(t *testing.T) {
	const src = `inner() { local x=I; echo "inner=[$x]"; }
outer() { local -r x=O; inner; echo "outer=[$x]"; }
outer`
	out, errs, st := declRun(t, src, enclosingFreeze(No), Diagnostics{})
	want := "inner=[O]\nouter=[O]\n"
	if out != want || st != 0 {
		t.Errorf("refusing over a caller's freeze = %q (status %d), want %q", out, st, want)
	}
	if !strings.Contains(errs, "x: readonly variable") {
		t.Errorf("stderr = %q, want the declaration's refusal", errs)
	}
}

// The running scope's own freeze is refused under *either* answer: the
// second declaration is writing the cell the first one froze, so there is no
// enclosing binding for it to stand in front of. This is the control that
// keeps the narrower answer from reading as "a local is never refused".
func TestAScopesOwnFreezeIsRefusedWhicheverWayTheAxisGoes(t *testing.T) {
	const src = `a() { local -r x=1; local x=2; echo "a=[$x]"; }
a`
	for _, ans := range []Answer{Yes, No} {
		out, errs, st := declRun(t, src, enclosingFreeze(ans), Diagnostics{})
		want := "a=[1]\n"
		if out != want || st != 0 {
			t.Errorf("a redeclaration under %v = %q (status %d), want %q", ans, out, st, want)
		}
		if !strings.Contains(errs, "x: readonly variable") {
			t.Errorf("stderr under %v = %q, want the declaration's refusal", ans, errs)
		}
	}
}

// But a **letter** on the scope's own frozen cell is not a shadow question at
// all, and the scope must not answer it.
//
// The row above is the control beside this one and they part on one thing: a
// declaration carrying a *value* for a frozen local is refused by the store,
// and one carrying only a letter has no store to be refused by. The shadow
// check stood in front of both, so inside a function an attribute letter was
// unreachable on a name that function had frozen itself — while the same line
// at the top level was taken.
//
// `+n` is the letter that shows it, because it is the one that is about the
// *reference* rather than about the value the freeze guards. Measured
// 2026-09-24 on bash 5.3.20, both at 0 and both leaving `declare -r v`:
//
//	declare -r -n v; declare +n v
//	f(){ declare -r -n v; declare +n v; }
//
// This shell took the first and refused the second (#4178).
func TestALetterOnTheScopesOwnFrozenCellIsNotAShadow(t *testing.T) {
	const src = `a() { typeset -r -n v; typeset +n v; echo "st=[$?]"; typeset -p v; }
a`
	for _, ans := range []Answer{Yes, No} {
		out, errs, st := declRun(t, src, func(s *Semantics) {
			enclosingFreeze(ans)(s)
			// The reference letter and the listing letter, without which the
			// row is an invalid option and measures nothing at all.
			s.LocalOptions = "aAilnprux"
			s.DeclareOptions = "aAilnprux"
			// `-r` beside `-n` on one line, which is a question of its own
			// and not this row's: answered flat so an unanswered axis cannot
			// stand in for the freeze the row is about. See
			// Semantics.NamerefLetterStandsAlone.
			s.NamerefLetterStandsAlone = No
		}, Diagnostics{})
		want := "st=[0]\ndeclare -r v\n"
		if out != want || st != 0 || errs != "" {
			t.Errorf("a letter on the scope's own freeze under %v = %q (stderr %q, status %d), want %q",
				ans, out, errs, st, want)
		}
	}
}

// And a freeze no scope holds is refused under either answer too, at every
// depth — because the first refusal leaves no shadow for the second call to
// be enclosed by.
func TestATopLevelFreezeIsRefusedAtEveryDepth(t *testing.T) {
	const src = `readonly g=G
p1() { local g=L1; echo "p1=[$g]"; }
p2() { local g=L2; p1; }
p2`
	for _, ans := range []Answer{Yes, No} {
		out, errs, st := declRun(t, src, enclosingFreeze(ans), Diagnostics{})
		want := "p1=[G]\n"
		if out != want || st != 0 {
			t.Errorf("a top-level freeze under %v = %q (status %d), want %q", ans, out, st, want)
		}
		if n := strings.Count(errs, "g: readonly variable"); n != 2 {
			t.Errorf("stderr under %v = %q, want both declarations refused", ans, errs)
		}
	}
}
