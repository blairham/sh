// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestRatchetMovesOnlyOneWay: a failure off the list and a pass on it both
// trip the ratchet, a listed failure does not, and a listed case the run did
// not grade is left alone so that -only checks its own slice (#5710).
func TestRatchetMovesOnlyOneWay(t *testing.T) {
	rep := &Report{}
	for _, m := range []Match{
		{CaseID: "a/still-failing"},
		{CaseID: "b/newly-failing"},
		{CaseID: "c/newly-passing", OK: true},
		{CaseID: "d/passing", OK: true},
	} {
		rep.Matches = append(rep.Matches, m)
	}
	regressed, fixed := Ratchet(rep, []string{"a/still-failing", "c/newly-passing", "z/not-graded"})
	if !reflect.DeepEqual(regressed, []string{"b/newly-failing"}) {
		t.Errorf("regressed = %v", regressed)
	}
	if !reflect.DeepEqual(fixed, []string{"c/newly-passing"}) {
		t.Errorf("fixed = %v", fixed)
	}
}

// TestABaselineRoundTrips: what WriteBaseline writes, LoadBaseline reads back
// as exactly the failing set, comments and all; and a missing file is an
// error, never the empty list that would claim every case passes.
func TestABaselineRoundTrips(t *testing.T) {
	rep := &Report{Matches: []Match{{CaseID: "x/two"}, {CaseID: "x/one"}, {CaseID: "x/ok", OK: true}}}
	path := filepath.Join(t.TempDir(), "b.txt")
	if err := WriteBaseline(path, "dash", rep); err != nil {
		t.Fatal(err)
	}
	got, err := LoadBaseline(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"x/one", "x/two"}) {
		t.Errorf("round trip = %v", got)
	}
	if regressed, fixed := Ratchet(rep, got); len(regressed)+len(fixed) != 0 {
		t.Errorf("a fresh baseline does not hold: %v %v", regressed, fixed)
	}
	if _, err := LoadBaseline(filepath.Join(t.TempDir(), "missing.txt")); !os.IsNotExist(err) {
		t.Errorf("missing baseline: err = %v, want not-exist", err)
	}
}
