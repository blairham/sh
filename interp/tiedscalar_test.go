// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The substrate behind `typeset -T`: a letter the dialect hands the builtin
// through Semantics.DeclareOptions, a pair of names recorded against each
// other, and every write to either half mirrored into the other. Tests name
// axes and letters, never shells.

// withTies is declRun's setter for a dialect that spells the letter.
func withTies(s *Semantics) {
	s.DeclareOptions = "aAgHilpruUTx"
	s.LocalOptions = "aAHilpruUTx"
	s.DeclareListing = DeclareListingExportSpelled
	s.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	s.BareDeclarationListing = DeclareListingPlainAssignment
	s.ArraysAreSparse = No
	s.ArrayBaseIsZero = No
}

// The mirror, in both directions, at every place a write can happen: the
// scalar, the array, an append and one element. A one-directional mirror
// passes the first line and fails the second.
func TestATiedScalarAndArrayMirrorEachOther(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=a:b:c
echo "1 n=${#s[@]} [${s[@]}]"
s=(x y z)
echo "2 S=[$S]"
s+=(w)
echo "3 S=[$S]"
s[1]=Z
echo "4 S=[$S]"
S=p:q
echo "5 [${s[@]}]"`, withTies, Diagnostics{})
	want := "1 n=3 [a b c]\n2 S=[x:y:z]\n3 S=[x:y:z:w]\n4 S=[Z:y:z:w]\n5 [p q]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a tie = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The mirrors must not call each other forever, which is the one way this
// shape can fail catastrophically rather than wrongly. A write through each
// half, and then a read of both, is what says the guard let go again.
func TestTheMirrorsDoNotChaseEachOther(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=a:b
s=(c d)
S=e:f
s=(g)
echo "S=[$S] s=[${s[@]}]"`, withTies, Diagnostics{})
	want := "S=[g] s=[g]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("alternating writes = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The separator, used in both directions, and it is the third operand rather
// than a flag.
func TestATieSplitsAndJoinsOnItsSeparator(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s '#'
S=a#b#c
echo "split=[${s[@]}]"
s=(1 2)
echo "join=[$S]"`, withTies, Diagnostics{})
	want := "split=[a b c]\njoin=[1#2]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a separator = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// `unset` of either half unsets both, and forgets the tie: a later
// assignment is a plain scalar again.
func TestUnsettingHalfATieUnsetsAllOfIt(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
S=a:b
unset S
echo "1 n=${#s[@]} s=[${s[@]-UNSET}]"
typeset -T T2 t2
T2=a:b
unset t2
echo "2 T2=[${T2-UNSET}]"
typeset -T U u
U=a:b
unset U
U=c:d
echo "3 untied n=${#u[@]} U=[$U]"`, withTies, Diagnostics{})
	want := "1 n=0 s=[UNSET]\n2 T2=[UNSET]\n3 untied n=0 U=[c:d]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("unsetting a tie = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A declaration with no value leaves the scalar set and empty and the array
// with *no* elements — which is not the same as an empty string assigned on
// purpose, because that splits into one field. The mirror is held off at the
// declaration for exactly that reason.
func TestAValuelessTieLeavesTheArrayWithNoElements(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T S s
echo "1 S=[${S-UNSET}] n=${#s[@]}"
S=a
echo "2 n=${#s[@]}"
S=a:b
echo "3 n=${#s[@]}"`, withTies, Diagnostics{})
	want := "1 S=[] n=0\n2 n=1\n3 n=2\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a valueless tie = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// A name that already holds a value keeps it and is read back through the tie
// that has just arrived, the same rule the integer and case attributes
// follow. Emptying it instead is the failure this guards: it is what a tie
// made over `PATH` would do to `PATH`.
func TestATieReadsBackWhatTheScalarAlreadyHeld(t *testing.T) {
	out, errs, st := declRun(t, `V=one:two:three
typeset -T V v
echo "n=${#v[@]} [${v[@]}] V=[$V]"`, withTies, Diagnostics{})
	want := "n=3 [one two three] V=[one:two:three]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("a standing value = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// The refusals that are reported and run on, each naming what it refused.
func TestTheReportedTieRefusals(t *testing.T) {
	out, errs, st := declRun(t, `typeset -T X
echo "1 st=$?"
typeset -T S s
typeset -T S other
echo "2 st=$?"
typeset -T S s
echo "3 st=$?"
typeset -T Q q=plain
echo "4 st=$?"`, withTies, Diagnostics{})
	wantOut := "1 st=1\n2 st=1\n3 st=0\n4 st=1\n"
	wantErr := "testsh: -T requires names of scalar and array\n" +
		"testsh: can't tie already tied scalar: S\n" +
		"testsh: second argument of tie must be array: q\n"
	if out != wantOut || errs != wantErr || st != 0 {
		t.Errorf("the refusals = %q (stderr %q, status %d), want %q and %q",
			out, errs, st, wantOut, wantErr)
	}
}

// Tying a name to itself is refused, and the *fatality* is the dialect's
// answer about a refused declaration operand rather than this letter's own —
// which is what keeps one rule instead of two.
func TestASelfTieTakesTheDialectsAnswerAboutFatality(t *testing.T) {
	fatal := func(s *Semantics) { withTies(s); s.BadNameToDeclarationFatal = Yes }
	out, errs, st := declRun(t, `typeset -T A A
echo "reached"`, fatal, Diagnostics{})
	if out != "" || errs != "testsh: can't tie a variable to itself: A\n" || st != 1 {
		t.Errorf("fatal = %q (stderr %q, status %d), want nothing with 1", out, errs, st)
	}
	notFatal := func(s *Semantics) { withTies(s); s.BadNameToDeclarationFatal = No }
	out, errs, st = declRun(t, `typeset -T A A
echo "reached"`, notFatal, Diagnostics{})
	if out != "reached\n" || errs != "testsh: can't tie a variable to itself: A\n" || st != 0 {
		t.Errorf("not fatal = %q (stderr %q, status %d), want `reached`", out, errs, st)
	}
}

// A half that is not a name goes through the refusal every declaration
// operand goes through, so the dialect's wording and fatality apply. It is
// also what keeps a reordered operand list from tying something nobody
// wrote: a parser that lifts an array literal out of the arguments and
// appends its bare name turns `typeset -T R r=(a b) ':'` into `R`, `:`, `r`.
func TestATieHalfMustBeAName(t *testing.T) {
	dg := Diagnostics{BuiltinBadName: map[string]string{"typeset": "not valid in this context: %[2]s"}}
	_, errs, st := declRun(t, `typeset -T A :`, withTies, dg)
	want := "testsh: not valid in this context: :\n"
	if errs != want || st != 1 {
		t.Errorf("a non-name half = %q (status %d), want %q with 1", errs, st, want)
	}
}

// The letters go to the halves they belong to: export to the scalar alone,
// because the scalar is what a child can be told, and the rest to both. The
// listing is where that shows, and `T` is written last of all the letters.
func TestATieListsBackWithTheLettersItsHalvesHave(t *testing.T) {
	out, errs, st := declRun(t, `typeset -TUx A a
a=(p q p)
typeset -p A a
typeset -Tr B b
typeset -p B b
typeset -T C c '#'
c=(1 2)
typeset -p C c`, withTies, Diagnostics{})
	want := "export -UT A a=( p q )\ntypeset -aUT A a=( p q )\n" +
		"typeset -rT B b=(  )\ntypeset -arT B b=(  )\n" +
		"typeset -T C c=( 1 2 ) '#'\ntypeset -aT C c=( 1 2 ) '#'\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("the tie listing = %q (stderr %q, status %d), want %q", out, errs, st, want)
	}
}

// Runner.Tie is the seam a *dialect* uses for the ties a shell arrives with:
// `typeset -T` without the flags, the scope or the refusals a builtin needs.
// Three things only it can say — the seeding of a standing value, the empty
// separator meaning the default, and that it confers no export attribute on
// the scalar it ties.
func TestTheTieSeamSeedsDefaultsAndConfersNoExport(t *testing.T) {
	sem := permissive()
	sem.DeclareOptions = "aAgHilpruUTx"
	sem.DeclareListing = DeclareListingExportSpelled
	sem.DeclareValueQuoting = ListingQuoteWhenNeededEscaped
	sem.ArraysAreSparse = No
	sem.ArrayBaseIsZero = No
	var out, errs bytes.Buffer
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
		Vars: map[string]string{"STANDING": "one:two:three"},
	})
	// An empty separator is the default one, which is what a caller that
	// does not care should be able to write.
	r.Tie("STANDING", "standing", "")
	r.Tie("FRESH", "fresh", ":")
	f, err := syntax.Parse(`echo "seeded n=${#standing[@]} [${standing[@]}]"
echo "fresh n=${#fresh[@]} FRESH=[${FRESH-UNSET}]"
typeset -p STANDING`, syntax.Core())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatalf("run: %v", err)
	}
	// `typeset` and not `export`: the tie carries whatever the scalar was,
	// and this one was never exported.
	want := "seeded n=3 [one two three]\nfresh n=0 FRESH=[]\n" +
		"typeset -T STANDING standing=( one two three )\n"
	if out.String() != want || errs.String() != "" {
		t.Errorf("the seam = %q (stderr %q), want %q", out.String(), errs.String(), want)
	}
}

// A letter outside the dialect's set is still refused by name, which is what
// keeps the tie a dialect's to have rather than the substrate's to assume.
func TestTheTieLetterIsTheDialectsToGive(t *testing.T) {
	_, errs, st := declRun(t, `typeset -T S s`, nil,
		Diagnostics{UnimplementedOptionLetters: map[string]string{"typeset": "T"}})
	want := "testsh: typeset: -T is not implemented yet\n"
	if errs != want || st != 2 {
		t.Errorf("typeset -T without the letter = %q (status %d), want %q with 2", errs, st, want)
	}
}
