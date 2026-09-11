// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A name that is unset **and** typed: what a valueless compound declaration
// leaves where DeclaredNameWithoutValueIsEmpty says no (#1664, #1554).
//
// Both facts are wanted at once and only one of them is a value — the name is
// unset for the purposes of `${q-UNSET}`, and it carries the kind for the
// purposes of a listing — and the engine had no way to say it: the declaration
// put an empty array in the store, and hiding the name deleted it again, so
// the attribute the declaration recorded had nowhere left to live.
//
// Named for the axis this hangs off and never for a shell.

func hiddenCompoundRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.TypesetLocalNeedsKeywordFunction = No
		// The answer that makes the question exist at all: a declared name
		// without a value is *unset* rather than empty.
		s.DeclaredNameWithoutValueIsEmpty = No
		s.ValuelessDeclarationHidesTheOuterValue = Yes
		// What `-p` does with a name it cannot find, which is the control
		// row's question and not this one's.
		s.DeclarePrintReportsAMissingName = Yes
	}, Diagnostics{})
}

// The headline: a valueless compound declaration still has a name to list, at
// status 0. The integer letter beside it is the control that was always
// right, which is what says this is the compound letters and not the listing.
func TestAHiddenNameKeepsTheCompoundAttribute(t *testing.T) {
	out, errs, st := hiddenCompoundRun(t,
		`f(){ local -a q; typeset -p q; echo "st=$?"; local -i n; typeset -p n; local -A m; typeset -p m; }; f`)
	want := "declare -a q\nst=0\ndeclare -i n\ndeclare -A m\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// And the name is still unset, which is the other half and the one a fix that
// merely stopped hiding would break.
func TestAHiddenTypedNameIsStillUnset(t *testing.T) {
	out, errs, st := hiddenCompoundRun(t,
		`f(){ local -a q; echo "[${q-UNSET}][${q+S}][${#q[@]}]"; }; f`)
	if want := "[UNSET][][0]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The caller's array comes back when the function returns, which is the
// control the whole shape rests on: the scope saved it, and keeping the
// declaration's own empty table in the store must not disturb that.
func TestTheCallersCompoundSurvivesAHiddenLocal(t *testing.T) {
	out, errs, st := hiddenCompoundRun(t,
		`arr=(a b c); f(){ local -a arr; echo "in=${#arr[@]}"; }; f; echo "out=${#arr[@]}"`)
	if want := "in=0\nout=3\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// A name `unset` took away is not a typed name: the attribute goes with the
// store there, and the listing has nothing to find. The row that says the new
// state belongs to the declaration rather than to every hidden name.
func TestAnUnsetCompoundIsNotListed(t *testing.T) {
	out, _, st := hiddenCompoundRun(t, `a=(x y); unset a; typeset -p a; echo "st=$?"`)
	if want := "st=1\n"; out != want || st != 0 {
		t.Errorf("got %q/%d, want %q", out, st, want)
	}
}

// Assigning to the hidden name brings it back as the kind it was declared,
// rather than as a scalar that happens to share the name.
func TestAssigningToAHiddenTypedNameFillsTheDeclaredKind(t *testing.T) {
	out, errs, st := hiddenCompoundRun(t, `f(){ local -a q; q=(x); typeset -p q; }; f`)
	if want := `declare -a q=([0]="x")` + "\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}
