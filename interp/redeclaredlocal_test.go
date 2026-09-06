// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A declaration without a value hides the outer value — and the value it hides
// has to be an *outer* one. A second declaration of a name its own scope has
// already declared stands in front of nothing but the local the first one
// made, and no shell throws that away.
//
// The question was asked of the *scope* rather than of the declaration: the
// name was in the scope's saved table, so the second line read as a shadow
// being taken and hid a value that was already local. Silent, at status 0, and
// destroying exactly what the function had just computed.

// redeclareRun answers the two axes that put a runner in front of the
// question: a valueless declaration brings the name into being unset, and it
// hides whatever was showing through.
func redeclareRun(t *testing.T, src string) (string, int) {
	t.Helper()
	return axisRun(t, src, func(s *Semantics) {
		s.DeclaredNameWithoutValueIsEmpty = No
		s.ValuelessDeclarationHidesTheOuterValue = Yes
		s.LocalInheritsTheExportAttribute = Yes
		s.LocalOptions = "x"
	})
}

// The headline: `local FOO=x; local FOO` leaves `x` standing.
func TestASecondLocalOfTheSameNameLeavesTheValueStanding(t *testing.T) {
	out, st := redeclareRun(t,
		`export FOO=bar; f() { local FOO=x; local FOO; echo "read=[${FOO-UNSET}]"; }; f; echo "after=[$FOO]"`)
	if want := "read=[x]\nafter=[bar]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The case this must not become, and the one the axis is really about: a
// *different* function's valueless declaration does hide the caller's local.
// Both directions in one test, because a fix that stopped hiding altogether
// would pass the test above and break this.
func TestAValuelessLocalInAnotherFunctionStillHidesTheCallersLocal(t *testing.T) {
	out, st := redeclareRun(t,
		`export FOO=bar; g() { local FOO; echo "read=[${FOO-UNSET}]"; }; `+
			`f() { local FOO=x; g; echo "back=[${FOO-UNSET}]"; }; f`)
	if want := "read=[UNSET]\nback=[x]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The `typeset` spelling reaches the same code by its own route — a keyword
// function, since that is the one all four shells with the builtin give a
// scope to.
func TestASecondTypesetOfTheSameNameLeavesTheValueStanding(t *testing.T) {
	out, st := redeclareRun(t,
		`export FOO=bar; function f { typeset FOO=x; typeset FOO; echo "read=[${FOO-UNSET}]"; }; f`)
	if want := "read=[x]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The third route: `-g` takes no shadow at all, so it is certainly not the
// declaration that put anything in front of the name — and the value went away
// anyway, because the scope had a shadow of its own from the line before.
func TestAGlobalDeclarationOverThisFunctionsLocalLeavesTheValueStanding(t *testing.T) {
	out, st := axisRun(t,
		`export FOO=bar; f() { local FOO=x; typeset -g FOO; echo "read=[${FOO-UNSET}]"; }; f; echo "after=[$FOO]"`,
		func(s *Semantics) {
			s.DeclaredNameWithoutValueIsEmpty = No
			s.ValuelessDeclarationHidesTheOuterValue = Yes
			s.LocalInheritsTheExportAttribute = Yes
			s.LocalOptions = "x"
			// The letter the core does not have: `-g` is the whole of this
			// route, so a dialect that spells it has to be asked.
			s.DeclareOptions = "gx"
		})
	if want := "read=[x]\nafter=[bar]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// A name the same declaration brings into being for the first time is still
// hidden — nothing is standing behind it, so there is nothing to hide, and it
// reads unset. The point is that the two names on one line are answered
// separately: `FOO` was declared before and keeps its value, `NEW` was not.
func TestOnlyTheNameItsOwnScopeAlreadyDeclaredKeepsItsValue(t *testing.T) {
	out, st := redeclareRun(t,
		`export FOO=bar; export NEW=out; f() { local FOO=x; local FOO NEW; `+
			`echo "read=[${FOO-UNSET}][${NEW-UNSET}]"; }; f`)
	if want := "read=[x][UNSET]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The valueless declaration first and the value second, which is the ordinary
// declare-then-assign shape and has to keep working: the assignment wins.
func TestAValueAfterAValuelessDeclarationOfTheSameNameStands(t *testing.T) {
	out, st := redeclareRun(t,
		`export FOO=bar; f() { local FOO; local FOO=y; echo "read=[${FOO-UNSET}]"; }; f`)
	if want := "read=[y]\n"; out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}
