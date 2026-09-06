// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A prelude function is the shell's own, and a listing of "what functions
// exist" is how a caller captures a *person's* state — so the two must not be
// the same set (#1035). The rule is #603's, asked by name instead of by
// declaration: nothing here sets a second mark, and every case below is the
// consequence of the one that was already there.

// preludeRun installs pre as the dialect's own text and then runs src on the
// same runner, the way a front end does — two runs, not one concatenation,
// because a prelude prepended to the snippet arrives as the script's own and
// the whole question here is whose a function is.
func preludeListingRun(t *testing.T, pre, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	sem := permissive()
	sem.DeclareOptions = "aAfFgiprx"
	sem.DeclareListing = DeclareListingClustered
	sem.DeclareValueQuoting = ListingQuoteAlwaysDouble
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &dg,
		Dir: t.TempDir(), Name: "testsh",
	})
	// One arrangement for every case here, the same one declRun uses: what
	// these are about is which functions a listing names, not how a body is
	// laid out, and the layouts belong to the dialects that set them.
	r.SetFunctionLayout(syntax.Layout{
		Indent: "  ", Nested: true, Lines: true,
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}, syntax.Layout{Indent: " ", Lines: true, BraceOpenSuffix: " "})
	pf, err := syntax.Parse(pre, syntax.Core())
	if err != nil {
		t.Fatalf("parse prelude %q: %v", pre, err)
	}
	r.SourcingPrelude(true)
	if _, perr := r.Run(context.Background(), pf); perr != nil {
		t.Fatalf("prelude %q: %v", pre, perr)
	}
	r.SourcingPrelude(false)
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// The prelude the cases below are graded against. Two functions and a helper
// with a name nobody would type, which is the shape that made the leak worth
// a bug rather than a note: `__helper` is not a name any shell has and not one
// anybody wrote, so a capture carrying it carries it from here.
const listingPrelude = "__helper() { echo helper; }\np() { __helper; }\nq() { echo q; }\n"

// TestAPreludeFunctionIsNotInAFunctionListing: the whole listing is the
// script's own functions and nothing else, in both of `-F`'s shapes.
func TestAPreludeFunctionIsNotInAFunctionListing(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "f() { echo f; }\ntypeset -F", nil)
	if want := "declare -f f\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q — the prelude's three are the shell's", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}

	// `-f` writes bodies, and the body is where a leaked name does its
	// damage: a capture sourced back defines it.
	out, errs, st = preludeListingRun(t, listingPrelude, "f() { echo f; }\ntypeset -f", nil)
	if want := "f () \n{ \n  echo f\n}\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
	for _, name := range []string{"__helper", "p", "q"} {
		if strings.Contains(out, name) {
			t.Errorf("stdout = %q, want no mention of the prelude's %s", out, name)
		}
	}
}

// TestAnEmptyFunctionListingIsStillZero: with nothing but the prelude defined
// the listing is empty and 0 — not 1, which is what a name that is no
// function answers, and not the prelude said back.
func TestAnEmptyFunctionListingIsStillZero(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "typeset -F", nil)
	if out != "" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want nothing written and 0", out, errs, st)
	}
}

// TestAPreludeFunctionAnswersWhenItIsNamed: hidden from the listing is not
// hidden from the shell. `type` already calls such a name a function (#603),
// and asking for it by name gets the same answer rather than a second one.
func TestAPreludeFunctionAnswersWhenItIsNamed(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "typeset -F p", nil)
	if out != "p\n" || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q status %d, want %q and 0", out, errs, st, "p\n")
	}

	out, errs, st = preludeListingRun(t, listingPrelude, "typeset -f q", nil)
	if want := "q () \n{ \n  echo q\n}\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
}

// TestRedefiningAPreludeFunctionPutsItInTheListing: the mechanism is #603's
// and it is compared by declaration, so a script that writes its own `p` owns
// the name from that moment — in the listing, and speaking for itself.
func TestRedefiningAPreludeFunctionPutsItInTheListing(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "p() { echo mine; }\ntypeset -F", nil)
	if want := "declare -f p\n"; out != want {
		t.Errorf("stdout = %q, want exactly %q — a redefinition is the script's own", out, want)
	}
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
}

// TestABareSetListingLeavesThePreludeOut: the other capture surface, reached
// by the other name. A bare `set` in the engine that lists functions after
// its variables asks the same question and had the same leak.
func TestABareSetListingLeavesThePreludeOut(t *testing.T) {
	out, errs, st := preludeListingRun(t, listingPrelude, "f() { echo f; }\nset", func(s *Semantics) {
		s.SetListing = SetListingAssignmentsThenFunctions
	})
	if errs != "" || st != 0 {
		t.Errorf("stderr %q status %d, want a silent 0", errs, st)
	}
	if want := "f () \n{ \n  echo f\n}\n"; !strings.Contains(out, want) {
		t.Errorf("stdout = %q, want the script's own function listed as %q", out, want)
	}
	for _, name := range []string{"__helper", "p () ", "q () "} {
		if strings.Contains(out, name) {
			t.Errorf("stdout = %q, want no mention of the prelude's %s", out, name)
		}
	}
}
