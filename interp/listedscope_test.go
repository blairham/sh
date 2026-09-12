// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A listed declaration saying **where it would land** — the `-g` word for a
// global written from inside a function, and the `local` command word for a
// local one. Part of DeclareListingExportSpelled's shape rather than an axis:
// see the constant, and #2041 for the measurements.
//
// Named for the form and never for a shell.

func scopeRun(t *testing.T, src string) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprxU"
		s.LocalOptions = "aAgiprxU"
		s.TypesetLocalNeedsKeywordFunction = No
		s.DeclareListing = DeclareListingExportSpelled
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}, Diagnostics{})
}

// The headline: one name, one listing, two places it is written from. The
// same state answers differently because the *answer* is about the reader's
// scope and not about the name.
func TestAListedGlobalSaysSoOnlyFromInsideAFunction(t *testing.T) {
	const src = `g=1; f(){ typeset -p g; }; f; typeset -p g`
	out, errs, st := scopeRun(t, src)
	if want := "typeset -g g=1\ntypeset g=1\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// And a local of the running function takes no letter, which is the other
// half of the same claim: a bare declaration already lands there.
func TestAListedLocalTakesNoScopeWord(t *testing.T) {
	out, errs, st := scopeRun(t, `f(){ local l=2; typeset -p l; }; f`)
	if want := "typeset l=2\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The *innermost* scope decides, not "some scope has it". A caller's local is
// not this function's to redeclare — a bare declaration here would shadow it
// rather than reach it — so the listing writes it as a global.
func TestACallersLocalListsAsAGlobal(t *testing.T) {
	out, errs, st := scopeRun(t, `inner(){ typeset -p L; }; outer(){ local L=1; inner; }; outer`)
	if want := "typeset -g L=1\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The letter is a word of its own ahead of the cluster, so a name carrying
// attributes keeps them where they were. Three kinds, because the cluster is
// built differently for each.
func TestTheScopeWordStandsAheadOfTheCluster(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`typeset -a a=(x y); f(){ typeset -p a; }; f`, "typeset -g -a a=( x y )\n"},
		{`typeset -r r=1; f(){ typeset -p r; }; f`, "typeset -g -r r=1\n"},
		{`typeset -A m=([k]=v); f(){ typeset -p m; }; f`, "typeset -g -A m=( [k]=v )\n"},
	} {
		out, errs, st := scopeRun(t, tc.src)
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%s: got %q/%d stderr %q, want %q", tc.src, out, st, errs, tc.want)
		}
	}
}

// The two command words that already say where they land do not take the
// letter, and they part company over the export letter itself: the word
// `export` **is** that letter and drops it, where `local` is a different fact
// and keeps it. Both halves in one test, because either alone reads as a
// fixed spelling.
func TestAnExportedNameSpellsItsScopeInTheCommandWord(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`export e=1; f(){ typeset -p e; }; f`, "export e=1\n"},
		{`f(){ local -x e=9; typeset -p e; }; f`, "local -x e=9\n"},
		{`f(){ local -x -r e=9; typeset -p e; }; f`, "local -rx e=9\n"},
		// An exported *compound* never earned the `export` spelling — the
		// word cannot carry the value — so it is the `typeset` row and takes
		// the letter, while its local counterpart is still `local`.
		{`typeset -a -x a=(1 2); f(){ typeset -p a; }; f`, "typeset -g -ax a=( 1 2 )\n"},
		{`f(){ local -a -x a=(1 2); typeset -p a; }; f`, "local -ax a=( 1 2 )\n"},
	} {
		out, errs, st := scopeRun(t, tc.src)
		if out != tc.want || st != 0 || errs != "" {
			t.Errorf("%s: got %q/%d stderr %q, want %q", tc.src, out, st, errs, tc.want)
		}
	}
}

// The other listing forms say nothing about scope, which is what makes this
// the ExportSpelled form's shape rather than a rule of the engine's. The same
// global read from the same place, under each of the other four.
func TestTheOtherListingFormsAreSilentAboutScope(t *testing.T) {
	for _, tc := range []struct {
		name string
		form DeclarationListingForm
		want string
	}{
		{"clustered", DeclareListingClustered, "declare -- g=1\n"},
		{"bare assignments", DeclareListingBareAssignments, "g=1\n"},
		{"command word", DeclareListingCommandWord, "typeset g=1\n"},
		{"plain assignment", DeclareListingPlainAssignment, "g=1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := declRun(t, `g=1; f(){ typeset -p g; }; f`, func(s *Semantics) {
				s.DeclareOptions = "aAgiprx"
				s.TypesetLocalNeedsKeywordFunction = No
				s.DeclareListing = tc.form
				s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
			}, Diagnostics{})
			if out != tc.want || st != 0 || errs != "" {
				t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, tc.want)
			}
		})
	}
}
