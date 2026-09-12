// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// *When* the case attributes act — once on the value being stored, or on
// every read of it. See Semantics.CaseAttributeFoldsWhenRead and #1755.
//
// Named for the axis. Every case below is written so that the two answers
// give different bytes, because the value a plain `$v` reads is the same
// under both and is not evidence about either.

func foldRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgilprux"
		s.LocalOptions = "aAilprux"
		s.CaseAttributeFoldsWhenRead = a
		// The re-read is its own axis and the last test here turns on it:
		// applying a letter to a name that already holds something has to
		// reach the store before the two answers can differ about what it
		// finds there.
		s.AttributeRereadsTheValueItFinds = Yes
		s.DeclareListing = DeclareListingExportSpelled
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
	}, Diagnostics{})
}

// The headline: the listing is the declaration that was read, or it is not.
// A shell that folds on the way in has no way back to the text the
// assignment carried.
func TestWhenTheCaseAttributeFoldsIsAnAxis(t *testing.T) {
	const src = `typeset -l lo=AB; typeset -u up=ab; typeset -p lo up; echo "[$lo][$up]"`
	out, errs, st := foldRun(t, src, Yes)
	if want := "typeset -l lo=AB\ntypeset -u up=ab\n[ab][AB]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("read: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	out, errs, st = foldRun(t, src, No)
	if want := "typeset -l lo=ab\ntypeset -u up=AB\n[ab][AB]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("stored: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// Taking the letter off is the sharper proof, because it asks the store
// directly: what is left is what was really being kept.
func TestTakingTheLetterOffRevealsWhatTheStoreHeld(t *testing.T) {
	const src = `typeset -l v=AB; typeset +l v; echo "[$v]"`
	out, _, st := foldRun(t, src, Yes)
	if want := "[AB]\n"; out != want || st != 0 {
		t.Errorf("read: got %q/%d, want %q", out, st, want)
	}
	out, _, st = foldRun(t, src, No)
	if want := "[ab]\n"; out != want || st != 0 {
		t.Errorf("stored: got %q/%d, want %q", out, st, want)
	}
}

// An append joins the **stored** text and not what a read of it answers,
// which is a third possible answer and the one this engine gave while the
// append went through the ordinary read: `abCD`, a text neither shell has.
func TestAnAppendJoinsTheStoredText(t *testing.T) {
	const src = `typeset -l lo=AB; lo+=CD; typeset -p lo; echo "[$lo]"`
	out, _, st := foldRun(t, src, Yes)
	if want := "typeset -l lo=ABCD\n[abcd]\n"; out != want || st != 0 {
		t.Errorf("read: got %q/%d, want %q", out, st, want)
	}
	out, _, st = foldRun(t, src, No)
	if want := "typeset -l lo=abcd\n[abcd]\n"; out != want || st != 0 {
		t.Errorf("stored: got %q/%d, want %q", out, st, want)
	}
}

// Every read is a read: a length, a slice, a pattern operator and a
// comparison all see the folded text, so the fold cannot be something one
// expansion route knows and another does not.
//
// The pattern row is the discriminating one, and it goes the other way from
// the rest: under the fold-on-read answer `${v/A/x}` finds nothing to match,
// because what the operator is given is already lower case.
func TestEveryReadOfAFoldedNameIsFolded(t *testing.T) {
	const src = `typeset -l v=AB; echo "${#v}[${v:0:1}][${v/A/x}][${v/a/x}]"; ` +
		`case $v in ab) echo low;; AB) echo up;; esac`
	for _, a := range []Answer{Yes, No} {
		out, _, st := foldRun(t, src, a)
		if want := "2[a][ab][xb]\nlow\n"; out != want || st != 0 {
			t.Errorf("%v: got %q/%d, want %q", a, out, st, want)
		}
	}
}

// Re-reading a value the name already held is the other side of the same
// claim: applying the letter to a name that is holding something changes the
// store under one answer and leaves it alone under the other.
func TestApplyingTheLetterToAHeldValue(t *testing.T) {
	const src = `v=AB; typeset -l v; typeset -p v; echo "[$v]"`
	out, _, st := foldRun(t, src, Yes)
	if want := "typeset -l v=AB\n[ab]\n"; out != want || st != 0 {
		t.Errorf("read: got %q/%d, want %q", out, st, want)
	}
	out, _, st = foldRun(t, src, No)
	if want := "typeset -l v=ab\n[ab]\n"; out != want || st != 0 {
		t.Errorf("stored: got %q/%d, want %q", out, st, want)
	}
}
