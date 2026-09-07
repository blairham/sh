// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `unset` takes a name's attributes away with the name.
//
// A rule and not an axis: every shell in the panel that spells a letter at
// all answers the same way, so these tests name letters and never shells. The
// attribute maps are keyed by name, so the failure they guard against is one
// where the *new* value assigned after an `unset` is read through the *old*
// name's attribute — silently, and at status 0.

// withEveryAttributeLetter is declRun's setter for a dialect that spells all
// of them at once, which no single shell does. Each letter is measured
// somewhere in the panel; putting them on one vector is what lets one test
// say that the clearing is one step rather than one step per letter.
func withEveryAttributeLetter(s *Semantics) {
	s.DeclareOptions = "aAgHilprUux"
	s.LocalOptions = "aAHilprUux"
}

// The integer attribute, which is the one that changes what an assignment
// *means*: with it `n=3+4` is seven and without it four characters.
func TestUnsetTakesTheIntegerAttributeAway(t *testing.T) {
	out, errs, st := declRun(t, `typeset -i n=5
unset n
n=3+4
echo "[$n]"`, withEveryAttributeLetter, Diagnostics{})
	if out != "[3+4]\n" || st != 0 || errs != "" {
		t.Errorf("unset of an integer name = %q (stderr %q, status %d), want %q",
			out, errs, st, "[3+4]\n")
	}
}

// Both case letters, separately. They share one answer everywhere they are
// asked, and a per-letter clearing that forgot one would still pass a test
// that only asked about the other — which is how a letter goes unexercised.
func TestUnsetTakesEachCaseAttributeAway(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"upper", "typeset -u u=abc\nunset u\nu=def\necho \"[$u]\"", "[def]\n"},
		{"lower", "typeset -l l=ABC\nunset l\nl=GHI\necho \"[$l]\"", "[GHI]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, withEveryAttributeLetter, Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("unset of a folded name = %q (stderr %q, status %d), want %q",
					out, errs, st, tc.want)
			}
		})
	}
}

// The unique attribute, whose absence an append is what proves: with the
// letter on, the second `2` never lands.
func TestUnsetTakesTheUniqueAttributeAway(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U a=(1 1 2)
unset a
a=(3 3 4)
echo "[${a[@]}] n=${#a[@]}"`, withEveryAttributeLetter, Diagnostics{})
	want := "[3 3 4] n=3\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset of a unique name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The hiding attribute, which changes nothing about a read and only a
// listing — so the only way to see it go is to list the name again.
func TestUnsetTakesTheHidingAttributeAway(t *testing.T) {
	out, errs, st := declRun(t, `typeset -H h=hid
typeset -p h
unset h
h=shown
typeset -p h`, func(s *Semantics) {
		withEveryAttributeLetter(s)
		s.DeclareListing = DeclareListingExportSpelled
		s.BareDeclarationListing = DeclareListingPlainAssignment
		s.DeclarePrintReportsAMissingName = No
	}, Diagnostics{})
	want := "typeset h\ntypeset h=\"shown\"\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset of a hidden name = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The table attribute, which is not a flag but a kind: once it is gone the
// name takes a scalar, so the assignment is an assignment and not a
// non-numeric subscript.
func TestUnsetTakesTheTableAttributeAway(t *testing.T) {
	out, errs, st := declRun(t, `typeset -A m
m[k]=v
unset m
m=plain
echo "[$m] st=$?"`, withEveryAttributeLetter, Diagnostics{})
	want := "[plain] st=0\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset of a table = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The export attribute, read back through a child's environment — which is
// the only place its loss is visible at all. The shell's own reads answer
// the same either way, so a test that asked `$e` would pass against a shell
// that still hands the name to everything it starts.
func TestUnsetTakesTheExportAttributeAwayFromAChildsView(t *testing.T) {
	out, errs, st := declRunEnv(t, `typeset -x e=1
unset e
e=2
echo "own=[$e]"
env`, withEveryAttributeLetter, Diagnostics{}, testPATH())
	if st != 0 || errs != "" {
		t.Fatalf("unset of an exported name: stderr %q, status %d", errs, st)
	}
	if !strings.Contains(out, "own=[2]\n") {
		t.Errorf("out = %q, want the shell itself still reading the new value", out)
	}
	if strings.Contains(out, "e=1") || strings.Contains(out, "\ne=2") {
		t.Errorf("out = %q, want the child told nothing about the name", out)
	}
}

// The first of the two bounds: an element is not the name, so the attribute
// stands. Written through the unique letter because it is the one whose
// presence a later *write* can prove — every other letter would read the
// same whether it survived or not.
func TestUnsetOfAnElementLeavesTheAttributeStanding(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U u=(1 2 3)
unset 'u[1]'
u+=(3)
echo "n=${#u[@]}"`, func(s *Semantics) {
		withEveryAttributeLetter(s)
		s.UnsetTakesASubscript = Yes
		s.ArraysAreSparse = No
	}, Diagnostics{})
	want := "n=3\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unset of one element = %q (stderr %q, status %d), want %q — "+
			"the append is still the duplicate the attribute drops", out, errs, st, want)
	}
}

// The second bound: the refusal stands in front of the removal, so a frozen
// name keeps its value *and* everything it was declared to be. Nothing is
// cleared behind a refusal.
func TestUnsetOfAReadonlyNameClearsNothing(t *testing.T) {
	out, errs, st := declRun(t, `typeset -ri r=5
unset r
echo "st=$?"
echo "[$r]"
typeset -p r`, func(s *Semantics) {
		withEveryAttributeLetter(s)
		s.UnsetReadonlyFatal = No
	}, Diagnostics{UnsetReadonly: "%[1]s: cannot unset: readonly variable"})
	// The listing is what says the *attribute* survived: the value and the
	// status alone are the same for a name whose attributes were cleared
	// behind the refusal, which is a mutant this test used to let through.
	want := "st=1\n[5]\ndeclare -ir r=\"5\"\n"
	if out != want || st != 0 || errs == "" {
		t.Errorf("unset of a readonly name = %q (stderr %q, status %d), want %q with a refusal",
			out, errs, st, want)
	}
}

// The other record `unset` leaves behind, and one rule with the attributes
// rather than a second: the value is deleted *and* the name is marked gone,
// because a name that came from the environment is not in the table to
// delete from. So the mark has to be lifted by whatever brings the name
// back, and nothing lifted it — every read went on working, since a stored
// value answers ahead of the mark, and a listing was told the name is gone.
func TestANameAssignedAfterAnUnsetListsAgain(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", "typeset -i q=5\nunset q\nq=7\ntypeset -p q", "declare -- q=\"7\"\n"},
		{"a scalar with no attribute at all", "p=1\nunset p\np=zz\ntypeset -p p", "declare -- p=\"zz\"\n"},
		{"an indexed array", "a=(1 2)\nunset a\na=(3 4)\ntypeset -p a", "declare -a a=([0]=\"3\" [1]=\"4\")\n"},
		{"a keyed table", "typeset -A m\nm[k]=v\nunset m\ntypeset -A m\nm[j]=w\ntypeset -p m", "declare -A m=([j]=\"w\" )\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, tc.src, func(s *Semantics) {
				withEveryAttributeLetter(s)
				s.DeclarePrintReportsAMissingName = No
			}, Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("listing after unset then assign = %q (stderr %q, status %d), want %q",
					out, errs, st, tc.want)
			}
		})
	}
}
