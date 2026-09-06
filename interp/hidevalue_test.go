// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind `typeset -H`: a letter the dialect hands the builtin
// through Semantics.DeclareOptions, recorded against the name, and consulted
// by whatever lists it. Tests name axes and letters, never shells.

// withHiding is declRun's setter for a dialect that spells the letter, in the
// listing shape the one shell with it uses.
func withHiding(s *Semantics) {
	s.DeclareOptions = "aAgHilprux"
	s.LocalOptions = "aAHilprux"
	s.DeclareListing = DeclareListingExportSpelled
	s.ExportListing = DeclareListingCommandWord
	s.BareDeclarationListing = DeclareListingPlainAssignment
	s.SetListing = SetListingAssignments
	s.SetListingQuoting = ListingQuoteWhenNeededEscaped
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
}

// The whole of what the letter does. A read is untouched; the listing loses
// the value and keeps every attribute, and `+H` gives it back.
func TestTheHidingAttributeWithholdsTheValueFromADeclarationListing(t *testing.T) {
	out, errs, st := declRun(t, `typeset -H h=hid
echo "read=[$h]"
typeset -p h
typeset -iH n=5
typeset -p n
typeset +H n
typeset -p n`, withHiding, Diagnostics{})
	want := "read=[hid]\ntypeset h\ntypeset -i n\ntypeset -i n=5\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("typeset -H = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A letter outside the dialect's set is still refused, which is what keeps
// the attribute a dialect's to have rather than the substrate's to assume.
func TestTheHidingLetterIsTheDialectsToGive(t *testing.T) {
	_, errs, st := declRun(t, `typeset -H h=hid`, nil,
		Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "H"}})
	want := "testsh: typeset: -H is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("typeset -H without the letter = %q (status %d), want %q with 2", errs, st, want)
	}
}

// Compound names: the kind letters still list, the elements do not.
func TestTheHidingAttributeWithholdsACompoundValue(t *testing.T) {
	out, errs, st := declRun(t, `typeset -AH m
m[k]=v
echo "elem=[${m[k]}]"
typeset -p m`, withHiding, Diagnostics{})
	want := "elem=[v]\ntypeset -A m\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("typeset -AH = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The other three listings a hidden name can reach, each in its own shape and
// each short of the value where an ordinary name on the same listing keeps
// one. Whole lines, matched as lines: a `Contains` would find `export ex`
// inside `export ex2=E2` and call the value hidden when it was not.
func TestTheHidingAttributeReachesEveryListingThatWritesAValue(t *testing.T) {
	out, errs, st := declRun(t, `typeset -xH ex=E
typeset -x ex2=E2
typeset -H zzh=hid
zzv=plain
export -p
export
set`, withHiding, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("stderr %q status %d, want a clean listing", errs, st)
	}
	for _, want := range []string{
		"export ex",     // DeclareListingCommandWord, hidden
		"export ex2=E2", // and not hidden, on the same listing
		"ex",            // DeclareListingPlainAssignment, hidden
		"ex2=E2",
		"zzh",       // SetListingAssignments, hidden
		"zzv=plain", // and the proof the bare `set` ran at all
	} {
		if !hasWholeLine(out, want) {
			t.Errorf("no line %q in %q", want, out)
		}
	}
	for _, unwanted := range []string{"export ex=E", "ex=E", "zzh=hid"} {
		if hasWholeLine(out, unwanted) {
			t.Errorf("line %q in %q, want the value withheld", unwanted, out)
		}
	}
}

func hasWholeLine(out, line string) bool {
	for _, got := range strings.Split(out, "\n") {
		if got == line {
			return true
		}
	}
	return false
}

// `-g` says where a declaration lands; it does not say what the name *is*.
// The table attribute was dropped on that path, so `typeset -gA m` left `m`
// an indexed array and the next `m[k]=v` was a non-numeric subscript.
func TestTheGlobalLetterStillRecordsTheTableAttribute(t *testing.T) {
	out, errs, st := declRun(t, `typeset -gA m
m[k]=v
echo "elem=[${m[k]}]"
typeset -p m`, withHiding, Diagnostics{})
	want := "elem=[v]\ntypeset -A m=( [k]=v )\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("typeset -gA = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A declaration with no value gives the name the empty one where the axis
// says so — but only a name it *creates*. Adding an attribute to a name that
// already holds a value kept the letter and threw the value away, which made
// `typeset -H h` on an existing `h` destroy what it meant to hide.
//
// Whether what survives is also *re-read* through the attribute that has just
// arrived is a separate question with a separate answer — see
// AttributeRereadsTheValueItFinds and the test below — so the letter asked
// about here is one that says nothing about a value.
func TestAValuelessDeclarationKeepsAValueTheNameAlreadyHas(t *testing.T) {
	set := func(s *Semantics) {
		withHiding(s)
		s.DeclaredNameWithoutValueIsEmpty = Yes
	}
	out, errs, st := declRun(t, `h=hid
typeset -H h
typeset +H h
typeset -p h`, set, Diagnostics{})
	want := "typeset h=hid\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("an attribute added to a name with a value = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// An attribute that *does* speak to a value either re-reads what the name
// already holds or waits for the next assignment, and both answers lose
// something: one destroys text that is not an expression, the other leaves a
// name declared integer holding text that is not a number. Both sides
// asserted, because asserting one asserts a default.
//
// Not an assignment either way, so a readonly name is re-read rather than
// refused — `typeset -r r=1; typeset -i r` is 1 at status 0 under both.
func TestAnAttributeRereadingAStandingValueIsAnAxis(t *testing.T) {
	const src = `a=5+2
typeset -i a
echo "a=[$a]"
d=MiXeD
typeset -u d
echo "d=[$d]"
typeset -r r=1
typeset -i r
echo "r=[$r]"`
	for _, tc := range []struct {
		answer Answer
		want   string
	}{
		{Yes, "a=[7]\nd=[MIXED]\nr=[1]\n"},
		{No, "a=[5+2]\nd=[MiXeD]\nr=[1]\n"},
	} {
		set := func(s *Semantics) {
			withHiding(s)
			s.DeclaredNameWithoutValueIsEmpty = Yes
			s.AttributeRereadsTheValueItFinds = tc.answer
		}
		out, errs, st := declRun(t, src, set, Diagnostics{})
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%v: got %q (stderr %q, status %d), want %q", tc.answer, out, errs, st, tc.want)
		}
	}
}

// The evaluation must not happen at all where the dialect says the attribute
// waits, because this engine's evaluation complains out loud and the shell
// that waits says nothing. `08` is the shape that separates the two: a bad
// octal digit under `-i`, and bash reads `08` back in silence at 0.
func TestAnAttributeThatWaitsDoesNotEvaluateTheStandingValue(t *testing.T) {
	set := func(s *Semantics) {
		withHiding(s)
		s.DeclaredNameWithoutValueIsEmpty = Yes
		s.AttributeRereadsTheValueItFinds = No
	}
	out, errs, st := declRun(t, `FOO=08
typeset -i FOO
echo "read=[$FOO]"`, set, Diagnostics{})
	if want := "read=[08]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q (stderr %q, status %d), want %q and nothing said", out, errs, st, want)
	}
}

// The other side of the same rule: a name the declaration creates still gets
// the empty value, and the cell a function's shadow makes is created however
// much the caller held.
func TestAValuelessDeclarationOfANewCellIsEmpty(t *testing.T) {
	set := func(s *Semantics) {
		withHiding(s)
		s.DeclaredNameWithoutValueIsEmpty = Yes
		// A `typeset` in a POSIX-style function declares a local here, which
		// is what makes the shadow — and so the fresh cell — happen at all.
		s.TypesetLocalNeedsKeywordFunction = No
	}
	out, errs, st := declRun(t, `typeset -x fresh
echo "fresh=[${fresh-UNSET}]"
outer=5
f() { typeset -x outer; echo "in=[${outer-UNSET}]"; }
f
echo "after=[$outer]"`, set, Diagnostics{})
	want := "fresh=[]\nin=[]\nafter=[5]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a valueless declaration = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A refusal names the sign the word was written with. `declare` and `typeset`
// are the only place a shell spells an option with a plus, and a refusal that
// answered `-q` to a `typeset +q` was naming a word the script never wrote.
func TestARefusedOptionKeepsItsSign(t *testing.T) {
	_, errs, st := declRun(t, `typeset +q v`, nil,
		Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"})
	want := "testsh: typeset: +q: invalid option\n"
	if errs != want || st != 2 {
		t.Errorf("typeset +q = %q (status %d), want %q with 2", errs, st, want)
	}
	// And a minus word is still a minus word.
	_, errs, st = declRun(t, `typeset -q v`, nil,
		Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"})
	want = "testsh: typeset: -q: invalid option\n"
	if errs != want || st != 2 {
		t.Errorf("typeset -q = %q (status %d), want %q with 2", errs, st, want)
	}
}
