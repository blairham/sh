// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// bareListing runs src under the listing form that answers an empty indexed
// array from the array's history.
func bareListing(t *testing.T, src string) string {
	t.Helper()
	out, errs, st := declareRun(t, src, func(s *Semantics) {
		s.DeclareListing = DeclareListingBareAssignments
		s.ArrayBaseIsZero = Yes
	}, Diagnostics{})
	if errs != "" || st != 0 {
		t.Errorf("%s: stderr %q, status %d", src, errs, st)
	}
	return strings.TrimSuffix(out, "\n")
}

// An indexed array holding no element lists with no value part where nothing
// was ever written into it, and as one empty element at the first subscript
// where something was. It wrote `=()` for both, so the listing said the same
// thing about two states the shell tells apart — and a listing read back is
// the use that notices.
func TestAnEmptyIndexedArrayListsFromWhatItHasHeld(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Nothing was ever written in: no value part at all.
		{`typeset -a a; typeset -p a`, "typeset -a a"},
		{`typeset -a b=(); typeset -p b`, "typeset -a b"},
		{`typeset -ia n; typeset -p n`, "typeset -a -i n"},
		// An element was written and taken away again.
		{`typeset -a d; d[0]=x; unset 'd[0]'; typeset -p d`, "typeset -a d=([0]=)"},
		{`e=(x); unset 'e[0]'; typeset -p e`, "typeset -a e=([0]=)"},
		// Always the *first* subscript, whatever the element that went was
		// at — which is what says this is about the array being empty and
		// not about how a removal is recorded.
		{`typeset -a d; d[2]=x; unset 'd[2]'; typeset -p d`, "typeset -a d=([0]=)"},
		{`e=(x y z); unset 'e[0]' 'e[1]' 'e[2]'; typeset -p e`, "typeset -a e=([0]=)"},
		// And an array that has held an element and still holds one is
		// untouched, hole or no hole. The value quoting is the vector's
		// rather than this rule's — a dialect answers it elsewhere.
		{`e=(x y z); unset 'e[1]'; typeset -p e`, `typeset -a e=([0]="x" [2]="z")`},
		{`e=(x); unset 'e[0]'; e[3]=z; typeset -p e`, `typeset -a e=([3]="z")`},
	} {
		if got := bareListing(t, c.src); got != c.want {
			t.Errorf("%s = %q, want %q", c.src, got, c.want)
		}
	}
}

// A **table** is the empty pair however it got there, which is the branch
// next door: the two kinds are answered apart, so a rule written over
// "compound" would have moved one of them wrongly.
func TestAnEmptyTableStillListsTheEmptyPair(t *testing.T) {
	for _, src := range []string{
		`typeset -A m; typeset -p m`,
		`typeset -A m; m[k]=1; unset 'm[k]'; typeset -p m`,
	} {
		if got := bareListing(t, src); !strings.HasSuffix(got, "=()") {
			t.Errorf("%s = %q, want the empty pair", src, got)
		}
	}
}

// Taking the whole name away starts the record over, so a fresh declaration
// of the same spelling lists as one that has held nothing.
func TestUnsettingTheNameForgetsWhatTheArrayHeld(t *testing.T) {
	const src = `typeset -a d; d[0]=x; unset d; typeset -a d; typeset -p d`
	if got := bareListing(t, src); got != "typeset -a d" {
		t.Errorf("%s = %q, want the declared-only shape", src, got)
	}
}

// And a scope saves it with the two tables beside it, where the vector makes
// a declaration inside a call a local one: an array emptied in the callee
// must not leave the caller's name of the same spelling reading as one that
// has lost its elements, and the caller's own history must survive a callee
// that declares the name afresh. Whether a given call opens a scope at all is
// Semantics.KeywordGatesALocalScope's and not this rule's.
func TestAScopeSavesWhatTheCallersArrayHadHeld(t *testing.T) {
	const src = `typeset -a q; f() { typeset -a q=(z); unset 'q[0]'; }; f; typeset -p q`
	if got := bareListing(t, src); got != "typeset -a q" {
		t.Errorf("%s = %q, want the caller's own history", src, got)
	}
	// The other direction: what the caller held is still held afterwards.
	const kept = `typeset -a q; q[0]=v; unset 'q[0]'; f() { typeset -a q; }; f; typeset -p q`
	if got := bareListing(t, kept); got != "typeset -a q=([0]=)" {
		t.Errorf("%s = %q, want the caller's array still remembered", kept, got)
	}
}
