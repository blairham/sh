// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"strconv"
	"testing"
)

// TestTheRememberedCellsAreForgottenWhenTheSlabMoves: Runner.cell remembers
// the last two cells it found by position, and a compaction moves every
// position. The names deleted on the way to one are the last two the store
// asked about, and once the slab is compacted they are not in it at all.
func TestTheRememberedCellsAreForgottenWhenTheSlabMoves(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out})
	r.varsPut("kept", "k")
	for i := range 600 {
		r.varsPut("v"+strconv.Itoa(i), "x")
	}
	compacted := false
	for i := range 600 {
		before := len(r.cellSlab)
		r.varsDel("v" + strconv.Itoa(i))
		if len(r.cellSlab) < before {
			compacted = true
			// The two names asked about last: the one just deleted and the
			// one before it, both gone from the store now.
			for _, gone := range []string{"v" + strconv.Itoa(i), "v" + strconv.Itoa(i-1)} {
				if c := r.cell(gone); c != nil {
					t.Fatalf("after the slab was compacted, %s answers with the cell of %q", gone, c.name)
				}
			}
			break
		}
	}
	if !compacted {
		t.Fatal("the slab was never compacted, so nothing was tested")
	}
	if v, _ := r.varsLookup("kept"); v != "k" {
		t.Errorf("kept = %q after the compaction, want k", v)
	}
}

// TestTwoNamesInTurnCostNoProbes: a loop counting two names asks about each
// in turn, and with the last two cells remembered neither is a probe of the
// index once both have been found.
func TestTwoNamesInTurnCostNoProbes(t *testing.T) {
	var out bytes.Buffer
	r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out})
	r.varsPut("i", "0")
	r.varsPut("n", "0")
	before := r.cellProbes
	for range 10 {
		r.varsPut("i", "1")
		r.varsPut("n", "1")
	}
	if probes := r.cellProbes - before; probes != 0 {
		t.Errorf("twenty stores into two names probed the index %d times, want none", probes)
	}
}
