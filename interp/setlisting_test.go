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

// `set` with no arguments lists the variables — what exactly, and spelled
// how, are SetListingForm and SetListingQuoting's to answer.

func listRun(t *testing.T, src string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	sem := permissive()
	if set != nil {
		set(&sem)
	}
	var out, errs bytes.Buffer
	r := newTestRunner(t, &Runner{
		Stdout: &out, Stderr: &errs, Semantics: &sem, Diagnostics: &Diagnostics{},
		Dir: t.TempDir(), Name: "testsh",
	})
	r.SetFunctionLayout(syntax.Layout{
		Indent: "  ", Nested: true, Lines: true,
		BraceOpenSuffix: " ", OutermostBraceOpensALine: true,
	}, syntax.Layout{})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return out.String(), errs.String(), st
}

// TestSetListsAssignmentsInTheDialectsQuoting: sorted `name=value` lines,
// each value spelled by SetListingQuoting — bare until it needs quoting in
// one style, always quoted with the embedded quote doubled out in another,
// `$'...'` in a third.
func TestSetListsAssignmentsInTheDialectsQuoting(t *testing.T) {
	src := "zz=plain\naa='has space'\nqq=\"quo'te\"\nset"
	tests := []struct {
		name  string
		style ListingQuotingStyle
		plain string
		space string
		quote string
	}{
		{
			"when needed escaped", ListingQuoteWhenNeededEscaped,
			"zz=plain\n", "aa='has space'\n", `qq='quo'\''te'` + "\n",
		},
		{
			"always doubled", ListingQuoteAlwaysDoubled,
			"zz='plain'\n", "aa='has space'\n", `qq='quo'"'"'te'` + "\n",
		},
		{
			"when needed dollar", ListingQuoteWhenNeededDollar,
			"zz=plain\n", "aa='has space'\n", `qq=$'quo\'te'` + "\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := listRun(t, src, func(s *Semantics) {
				s.SetListing = SetListingAssignments
				s.SetListingQuoting = tc.style
			})
			if st != 0 || errs != "" {
				t.Fatalf("status %d stderr %q, want a clean listing", st, errs)
			}
			for _, want := range []string{tc.plain, tc.space, tc.quote} {
				if !strings.Contains(out, want) {
					t.Errorf("stdout = %q, want %q in it", out, want)
				}
			}
			if strings.Index(out, "aa=") > strings.Index(out, "zz=") {
				t.Errorf("stdout = %q, want the names sorted", out)
			}
		})
	}
}

// TestSetListingCanFollowWithTheFunctions: one form appends every defined
// function after the variables, laid out the way the engine says functions
// back; the other stops at the variables.
func TestSetListingCanFollowWithTheFunctions(t *testing.T) {
	src := "v=1\nmyfn() { echo hi; }\nset"
	out, _, _ := listRun(t, src, func(s *Semantics) {
		s.SetListing = SetListingAssignmentsThenFunctions
		s.SetListingQuoting = ListingQuoteWhenNeededEscaped
	})
	if !strings.Contains(out, "echo hi") {
		t.Errorf("stdout = %q, want the function's body after the variables", out)
	}
	if strings.Index(out, "v=1") > strings.Index(out, "myfn") {
		t.Errorf("stdout = %q, want the variables first", out)
	}

	out, _, _ = listRun(t, src, func(s *Semantics) {
		s.SetListing = SetListingAssignments
		s.SetListingQuoting = ListingQuoteWhenNeededEscaped
	})
	if strings.Contains(out, "myfn") {
		t.Errorf("stdout = %q, want no functions in this form", out)
	}
}

// TestSetListingSpellsArraysTheWayDeclareDoes: the shape follows the
// dialect's `declare -p` and not its `set` quoting — subscripted
// double-quoted elements where declarations cluster, dense between padding
// spaces where an export is spelled out, and dense with no padding otherwise.
// Measured from `a=(x 'y z')` in all three.
func TestSetListingSpellsArraysTheWayDeclareDoes(t *testing.T) {
	src := "a=(x 'y z')\nset"
	for _, tc := range []struct {
		name string
		form DeclarationListingForm
		set  ListingQuotingStyle
		val  ListingQuotingStyle
		want string
	}{
		{
			"clustered", DeclareListingClustered, ListingQuoteWhenNeededEscaped,
			ListingQuoteAlwaysDouble, `a=([0]="x" [1]="y z")` + "\n",
		},
		{
			"bare", DeclareListingBareAssignments, ListingQuoteWhenNeededDollar,
			ListingQuoteWhenNeededDollar, "a=(x 'y z')\n",
		},
		{
			"export-spelled", DeclareListingExportSpelled, ListingQuoteWhenNeededEscaped,
			ListingQuoteWhenNeededEscaped, "a=( x 'y z' )\n",
		},
	} {
		out, _, _ := listRun(t, src, func(s *Semantics) {
			s.SetListing = SetListingAssignments
			s.SetListingQuoting = tc.set
			s.DeclareListing = tc.form
			s.DeclareValueQuoting = tc.val
		})
		if !strings.Contains(out, tc.want) {
			t.Errorf("%s: stdout = %q, want %q in it", tc.name, out, tc.want)
		}
	}
}

// TestSetListingRefusals: no answer at all is refused as the unanswered
// question it is.
func TestSetListingRefusals(t *testing.T) {
	_, errs, st := listRun(t, "set", func(s *Semantics) {
		s.SetListing = SetListingUnspecified
	})
	if st != 2 || !strings.Contains(errs, "no dialect was chosen") {
		t.Errorf("status %d stderr %q, want the unanswered-question refusal", st, errs)
	}
}
