// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// A listing that keeps the word the declaration was written with — see
// Diagnostics.FunctionListingKeywordHeader and its names-only half, and
// #1494.
//
// Named for the wording. The whole point of the pair is that a dialect may
// need the two spellings to stay apart; a dialect that answers only the first
// writes one header for both, which is what the fallback here asserts.

func headerRun(t *testing.T, src string, dg Diagnostics) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "afgprx"
		s.FunctionNamesUnderPlus = Yes
	}, dg)
}

// The headline: one listing, two functions, and the header follows how each
// was declared. The pair in one listing is what makes it visible — either row
// alone reads as a fixed spelling.
func TestAFunctionListingCanKeepTheDeclarationsKeyword(t *testing.T) {
	const src = "f(){ :; }\nfunction g { :; }\ntypeset -f\n"
	out, errs, st := headerRun(t, src, Diagnostics{
		FunctionListingHeader:        "%[1]s()%[2]s",
		FunctionListingKeywordHeader: "function %[1]s %[2]s",
	})
	// The body is the harness's own arrangement and not this test's
	// subject; what is asserted is the header in front of each.
	if want := "f(){ \n  :\n}\nfunction g { \n  :\n}\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// With no keyword wording answered, one header serves both — which is three
// of the four dialects, and is what keeps this from being a question every
// dialect has to answer.
func TestOneHeaderServesBothSpellingsByDefault(t *testing.T) {
	const src = "f(){ :; }\nfunction g { :; }\ntypeset -f\n"
	out, errs, st := headerRun(t, src, Diagnostics{FunctionListingHeader: "%[1]s()%[2]s"})
	if want := "f(){ \n  :\n}\ng(){ \n  :\n}\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The names-only listing makes the same distinction with punctuation instead
// of a word, and falls back the same way.
func TestANamesOnlyListingCanSpellTheDeclarationToo(t *testing.T) {
	const src = "f(){ :; }\nfunction g { :; }\ntypeset +f\n"
	out, _, st := headerRun(t, src, Diagnostics{
		FunctionNameListing:        "%[1]s()",
		FunctionNameListingKeyword: "%[1]s",
	})
	if want := "f()\ng\n"; out != want || st != 0 {
		t.Errorf("spelled: got %q/%d, want %q", out, st, want)
	}
	out, _, st = headerRun(t, src, Diagnostics{})
	if want := "f\ng\n"; out != want || st != 0 {
		t.Errorf("bare: got %q/%d, want %q", out, st, want)
	}
}
