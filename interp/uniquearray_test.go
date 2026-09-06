// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The substrate behind `typeset -U`: a letter the dialect hands the builtin
// through Semantics.DeclareOptions, recorded against the name, and consulted
// by every write to it. Tests name axes and letters, never shells.

// withUniqueness is declRun's setter for a dialect that spells the letter, in
// the listing shape and the array reading the one shell with it uses.
func withUniqueness(s *Semantics) {
	s.DeclareOptions = "aAgHilpruUx"
	s.LocalOptions = "aAHilpruUx"
	s.DeclareListing = DeclareListingExportSpelled
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	// The letter's shell reads an array densely, which is what decides what
	// a gap dedupes to — see the sparse test below.
	s.ArraysAreSparse = No
	s.ArrayBaseIsZero = No
}

// The whole of what the letter does: the first occurrence of each element
// survives, on the declaration and on every later write. A read-time dedupe
// would pass the first two lines and fail the third, because `+U` leaves the
// elements alone.
func TestTheUniqueAttributeKeepsTheFirstOfEachElement(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U a=(1 1 2 2 3 1)
echo "decl=[${a[@]}]"
a+=(2 4 4)
echo "append=[${a[@]}]"
typeset +U a
a+=(1)
echo "removed=[${a[@]}]"`, withUniqueness, Diagnostics{})
	want := "decl=[1 2 3]\nappend=[1 2 3 4]\nremoved=[1 2 3 4 1]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("typeset -U = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The first occurrence is the one kept, and it stays where it stands. A
// last-wins dedupe answers `1 2 3` to the same line, which is why the
// duplicate is put in *front* here rather than at the end.
func TestTheUniqueAttributeKeepsTheEarlierPlaceOfADuplicate(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U b=(1 2 3)
b=(3 "${b[@]}")
echo "prepended=[${b[@]}]"`, withUniqueness, Diagnostics{})
	want := "prepended=[3 1 2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a duplicate written in front = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// The attribute reaches what the name already holds. Applying it to an array
// that was built without it dedupes on the spot rather than waiting for the
// next assignment.
func TestTheUniqueAttributeDedupesWhatTheNameAlreadyHolds(t *testing.T) {
	out, errs, st := declRun(t, `c=(1 1 2)
typeset -U c
echo "applied=[${c[@]}]"`, withUniqueness, Diagnostics{})
	want := "applied=[1 2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the letter on an existing array = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// One element written dedupes the whole array and not only the element: a
// write that collides with an earlier one loses, and one that collides with
// a *later* one takes its place and the later one goes.
func TestTheUniqueAttributeDedupesAnElementWrite(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U d=(a b c)
d[2]=a
echo "collides-earlier=[${d[@]}]"
typeset -U e=(a b c)
e[1]=c
echo "collides-later=[${e[@]}]"`, withUniqueness, Diagnostics{})
	want := "collides-earlier=[a c]\ncollides-later=[c b]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("an element write under the letter = %q (stderr %q, status %d), want %q",
			out, errs, st, want)
	}
}

// A gap is elements, and they dedupe with everything else. The array is read
// the way the dialect reads it and stored back dense, so `f=(1 2); f[5]=1`
// leaves three — the two survivors and one empty from the two the gap made,
// with the trailing duplicate gone. Deduping the store's subscripts instead
// answers two elements and no empty at all.
func TestTheUniqueAttributeDedupesTheGapItIsRead(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U f=(1 2)
f[5]=1
echo "count=[${#f[@]}] elems=[${f[@]}]"`, withUniqueness, Diagnostics{})
	want := "count=[3] elems=[1 2 ]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a gap under the letter = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A scalar takes the letter and nothing happens to it: the attribute is about
// an array's elements, and a colon-joined string is not elements. Measured —
// the shell with the letter leaves `a:b:a` exactly as written.
func TestTheUniqueAttributeLeavesAScalarAlone(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U s=abcabc
echo "scalar=[$s]"
typeset -U p=a:b:a
echo "colons=[$p]"`, withUniqueness, Diagnostics{})
	want := "scalar=[abcabc]\ncolons=[a:b:a]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the letter on a scalar = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The letter comes back on the listing, last of all the letters — and the
// `export` spelling drops `x` from wherever it stands rather than off the
// end, which `U` standing after it is what exposed. Whole lines, because a
// substring assertion cannot see a letter added in front of the ones it
// looked for.
func TestTheUniqueAttributeListsBackAfterEveryOtherLetter(t *testing.T) {
	out, errs, st := declRun(t, `typeset -aUxr A1=(1 2)
typeset -p A1
typeset -Uxi n1=5
typeset -p n1
typeset -Ul s1=AB
typeset -p s1
typeset -Ux x1=v
typeset -p x1
typeset -AU m1
typeset -p m1`, withUniqueness, Diagnostics{})
	// `s1` lists back folded because this engine folds a case attribute at
	// assignment — see the note in applyDeclaration. The letters are what
	// this test is about.
	want := "typeset -arxU A1=( 1 2 )\nexport -iU n1=5\ntypeset -lU s1=ab\nexport -U x1=v\ntypeset -AU m1=( )\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the listed letters = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The attribute with no value of its own still makes the name listable, the
// way every other attribute does — otherwise `typeset -U u` followed by a
// bare listing loses the only thing that line did.
func TestAUniqueNameWithNoValueStillLists(t *testing.T) {
	out, errs, st := declRun(t, `typeset -U u
typeset -p`, withUniqueness, Diagnostics{})
	if st != 0 || errs != "" {
		t.Fatalf("stderr %q status %d, want a clean listing", errs, st)
	}
	// A whole line out of a listing the machine's own environment shares,
	// because a substring would find `typeset -U u` inside a line for a
	// name beginning `u`.
	if !hasWholeLine(out, "typeset -U u") {
		t.Errorf("no line %q in %q", "typeset -U u", out)
	}
}

// A letter outside the dialect's set is still refused by name, which is what
// keeps the attribute a dialect's to have rather than the substrate's to
// assume.
func TestTheUniqueLetterIsTheDialectsToGive(t *testing.T) {
	_, errs, st := declRun(t, `typeset -U a=(1 1)`, nil,
		Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "U"}})
	want := "testsh: typeset: -U is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("typeset -U without the letter = %q (status %d), want %q with 2", errs, st, want)
	}
}
