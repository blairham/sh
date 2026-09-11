// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// How far a `break` reaches is a question about *boundaries*, and the panel
// splits more widely here than anywhere else in the family: a call is a
// boundary in three columns and not in two, and the parentheses are one in a
// single column and in none of the others.
//
// The two are named as axes here and never as shells; which preset picks
// which is dialect/'s to assert.

const (
	breakThroughACall  = "f(){ break; }; for i in 1 2; do f; echo body; done; echo after"
	breakInASubshell   = "for i in 1 2; do ( break; echo insub ); echo body; done; echo after"
	breakTwoFromACall  = "f(){ for j in 1; do break 2; done; echo infunc; }; for i in 1 2; do f; echo body; done; echo after"
	breakTwoInSubshell = "for i in 1 2; do ( for j in 1; do break 2; done; echo insub ); echo body; done; echo after"
)

// TestFunctionCallIsALoopControlBoundary pins the axis by name: Yes leaves the
// caller's loop running and No ends it.
func TestFunctionCallIsALoopControlBoundary(t *testing.T) {
	for _, c := range []struct {
		boundary  Answer
		src, want string
	}{
		// A boundary, so the body's `break` has no loop to see: the word is
		// the misuse the neighboring axis answers, and the loop runs on.
		{Yes, breakThroughACall, "body body after"},
		{No, breakThroughACall, "after"},
		// The count is clamped at the boundary rather than refused: there is
		// a loop inside the body, so `break 2` ends that one and stops.
		{Yes, breakTwoFromACall, "infunc body infunc body after"},
		{No, breakTwoFromACall, "after"},
	} {
		sem := boundarySemantics()
		sem.FunctionCallIsALoopControlBoundary = c.boundary
		if got := boundaryRun(t, c.src, sem); got != c.want {
			t.Errorf("%v: %q gave %q, want %q", c.boundary, c.src, got, c.want)
		}
	}
}

// TestSubshellIsALoopControlBoundary pins the other one: Yes leaves the
// `break` with no loop, so the rest of the subshell runs; No lets it end the
// subshell's own copy of the loop and nothing more.
func TestSubshellIsALoopControlBoundary(t *testing.T) {
	for _, c := range []struct {
		boundary  Answer
		src, want string
	}{
		{Yes, breakInASubshell, "insub body insub body after"},
		{No, breakInASubshell, "body body after"},
		{Yes, breakTwoInSubshell, "insub body insub body after"},
		{No, breakTwoInSubshell, "body body after"},
	} {
		sem := boundarySemantics()
		sem.SubshellIsALoopControlBoundary = c.boundary
		if got := boundaryRun(t, c.src, sem); got != c.want {
			t.Errorf("%v: %q gave %q, want %q", c.boundary, c.src, got, c.want)
		}
	}
}

// TestTheTwoBoundariesAreIndependent is why they are two fields: the panel
// offers three of the four combinations, so no single field could express it.
func TestTheTwoBoundariesAreIndependent(t *testing.T) {
	for _, c := range []struct {
		call, sub Answer
		wantCall  string
		wantSub   string
	}{
		{Yes, Yes, "body body after", "insub body insub body after"},
		{Yes, No, "body body after", "body body after"},
		{No, No, "after", "body body after"},
	} {
		sem := boundarySemantics()
		sem.FunctionCallIsALoopControlBoundary = c.call
		sem.SubshellIsALoopControlBoundary = c.sub
		if got := boundaryRun(t, breakThroughACall, sem); got != c.wantCall {
			t.Errorf("call=%v sub=%v: through a call gave %q, want %q", c.call, c.sub, got, c.wantCall)
		}
		if got := boundaryRun(t, breakInASubshell, sem); got != c.wantSub {
			t.Errorf("call=%v sub=%v: in a subshell gave %q, want %q", c.call, c.sub, got, c.wantSub)
		}
	}
}

// TestABoundaryIsAskedAboutOnlyWhereItDecides: a boundary with enough loops
// inside it never has to be looked past, so neither axis is consulted and an
// unanswered one costs nothing.
//
// It is asserted with both left Unspecified, which is the state that makes an
// unwanted question audible: `ask` writes a line naming the axis and refuses.
func TestABoundaryIsAskedAboutOnlyWhereItDecides(t *testing.T) {
	for _, src := range []string{
		"f(){ for j in 1 2; do break; echo unreached; done; echo infunc; }; for i in 1; do f; done; echo after",
		"for i in 1; do ( for j in 1 2; do break; echo unreached; done; echo insub ); done; echo after",
	} {
		got := boundaryRun(t, src, boundarySemantics())
		if strings.Contains(got, "disagree") || strings.Contains(got, "unreached") {
			t.Errorf("%q gave %q, want the inner loop broken with no question asked", src, got)
		}
	}
}

// boundarySemantics answers everything the rows below depend on except the two
// axes under test, which each case sets for itself.
func boundarySemantics() Semantics {
	sem := PosixSemantics()
	// A `break` that ends up with no loop at all is the neighboring
	// question. Silent and survivable here, so that what the rows show is
	// where the word reached and not how the misuse is reported.
	sem.LoopControlOutsideALoopIsFatal = No
	return sem
}

func boundaryRun(t *testing.T, src string, sem Semantics) string {
	t.Helper()
	var buf strings.Builder
	dg := Diagnostics{}
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf,
		Semantics: &sem, Diagnostics: &dg, Name: "sh",
	})
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}
