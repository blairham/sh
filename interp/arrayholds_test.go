// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// ArrayHolds answers what listing the array and searching it answers.
//
// It is a shortcut — a stored array with no gaps is read straight out of the
// map, because membership does not care about the order the long way works
// out — so what it owes is agreement with the long way, on every shape an
// array comes in. A differential test rather than a table of expectations:
// the long way is the definition, and writing the answers out again by hand
// would only record what somebody believed twice.
func TestArrayHoldsAgreesWithListingTheArray(t *testing.T) {
	// The shapes, built by running shell rather than by filling the map, so
	// that what is under test is an array the interpreter made.
	const build = `
dense=(alpha beta gamma)
one=(only)
empty=()
withgap=(a b c)
unset 'withgap[2]'
scalar=plain
`
	probes := []string{"alpha", "beta", "gamma", "only", "plain", "a", "b", "c", "", "missing", "ALPHA"}
	names := []string{"dense", "one", "empty", "withgap", "scalar", "neverset"}

	r := newHoldsRunner(t)
	p := syntax.NewParser(build, syntax.Core())
	file := p.Parse()
	if err := p.Err(); err != nil {
		t.Fatal(err)
	}
	if st, err := r.Run(t.Context(), file); err != nil || st != 0 {
		t.Fatalf("building the arrays reported %d, %v", st, err)
	}

	agreed, found := 0, 0
	for _, name := range names {
		elems, ok := r.GetArray(name)
		for _, probe := range probes {
			want := ok && slices.Contains(elems, probe)
			got := r.ArrayHolds(name, probe)
			if got != want {
				t.Errorf("ArrayHolds(%q, %q) = %v, listing it says %v (elements %q)", name, probe, got, want, elems)
			}
			agreed++
			if want {
				found++
			}
		}
	}
	// Two controls. A run where nothing was ever found would agree on every
	// row by answering false to all of them, and a run against an empty name
	// list would agree on nothing at all — both read exactly like a pass.
	if found == 0 {
		t.Fatal("no probe was found in any array: this test compared false with false")
	}
	if agreed != len(names)*len(probes) {
		t.Fatalf("compared %d pairs, want %d", agreed, len(names)*len(probes))
	}
	t.Logf("%d pairs compared, %d of them a hit", agreed, found)
}

func newHoldsRunner(t *testing.T) *interp.Runner {
	t.Helper()
	sem := interp.PosixSemantics()
	d := syntax.Core()
	var out strings.Builder
	return newTestRunner(t, &interp.Runner{
		Semantics: &sem, Dialect: &d,
		Vars: map[string]string{}, Stdout: &out, Stderr: &out,
	})
}
