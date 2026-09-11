// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A valueless declaration writing the name back — see
// Semantics.ValuelessDeclarationOfAHeldNameListsIt and #1665.
//
// Named for the axis and never for a shell.

func listingRun(t *testing.T, src string, a Answer) (string, string, int) {
	t.Helper()
	return declRun(t, src, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.LocalOptions = "aAiprx"
		s.TypesetLocalNeedsKeywordFunction = No
		// The bare-assignment spelling this listing uses, and the value
		// quoting it hands to the renderer. Neither is what the axis asks;
		// both have to be answered for it to have anything to write.
		s.DeclareValueQuoting = ListingQuoteWhenNeededPlain
		s.ValuelessDeclarationOfAHeldNameListsIt = a
	}, Diagnostics{})
}

// The headline, both ways, and all three facts of the line at once: a name
// holding an array, a name holding a scalar, and — the control — a name
// holding nothing, which prints nothing under either answer.
func TestAValuelessDeclarationListingIsAnAxis(t *testing.T) {
	const src = `a=(x y); typeset a; s=str; typeset s; unset u; typeset u; echo done`
	out, errs, st := listingRun(t, src, Yes)
	if want := "a=( x y )\ns=str\ndone\n"; out != want || st != 0 || errs != "" {
		t.Errorf("yes: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
	out, errs, st = listingRun(t, src, No)
	if want := "done\n"; out != want || st != 0 || errs != "" {
		t.Errorf("no: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The value is untouched either way, which is why nothing caught this: the
// listing is the whole of what the declaration does here.
func TestAValuelessDeclarationListingChangesNothing(t *testing.T) {
	for _, a := range []Answer{Yes, No} {
		out, _, st := listingRun(t, `s=str; typeset s >/dev/null; echo "[$s]"`, a)
		if out != "[str]\n" || st != 0 {
			t.Errorf("%v: got %q/%d, want the value standing", a, out, st)
		}
	}
}

// A letter suppresses it. Each of these is a line the listing must *not*
// write, and each is a different reason: an attribute letter, the scope
// letter, and a word whose own name is an attribute.
func TestALetterSuppressesTheValuelessListing(t *testing.T) {
	for _, src := range []string{
		`n=5; typeset -i n`,
		`s=str; typeset -g s`,
		`a=(x y); typeset -a a`,
		`s=str; readonly s`,
		`s=str; export s`,
	} {
		out, errs, st := listingRun(t, src+`; echo after`, Yes)
		if out != "after\n" || st != 0 || errs != "" {
			t.Errorf("%s: got %q/%d stderr %q, want silence", src, out, st, errs)
		}
	}
}

// A **fresh** cell has nothing standing in it, which is what keeps a shell
// from narrating every declaration in every function; a second declaration of
// a name this scope has already made local does write it back.
func TestOnlyANameAlreadyInTheCellIsListed(t *testing.T) {
	out, errs, st := listingRun(t, `s=str; f(){ typeset s; }; f; echo after`, Yes)
	if out != "after\n" || st != 0 || errs != "" {
		t.Errorf("fresh: got %q/%d stderr %q, want silence", out, st, errs)
	}
	out, errs, st = listingRun(t, `f(){ local s=1; local s; echo mid; }; f`, Yes)
	if want := "s=1\nmid\n"; out != want || st != 0 || errs != "" {
		t.Errorf("already local: got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// The operand that *is* an assignment is not a valueless declaration, even
// though the builtin sees a bare name: the command machinery lands the value
// after the builtin returns, so listing here would write the old value out in
// front of a declaration that replaces it.
func TestAnOperandAssignmentIsNotListed(t *testing.T) {
	out, errs, st := listingRun(t, `b=(1 2); typeset b=(p q); printf '[%s]' "${b[@]}"; echo`, Yes)
	if want := "[p][q]\n"; out != want || st != 0 || errs != "" {
		t.Errorf("got %q/%d stderr %q, want %q", out, st, errs, want)
	}
}

// And the refusal is real where nothing answered, which is what says the
// silences above are decisions rather than a question nothing reaches.
func TestTheValuelessListingAxisIsAsked(t *testing.T) {
	out, errs, _ := declRun(t, `s=str; typeset s`, func(s *Semantics) {
		s.DeclareOptions = "aAgiprx"
		s.ValuelessDeclarationOfAHeldNameListsIt = Unspecified
	}, Diagnostics{})
	if !strings.Contains(errs, "a valueless declaration writing back a name that already holds something") {
		t.Errorf("got %q stderr %q, want a refusal naming the axis", out, errs)
	}
}
