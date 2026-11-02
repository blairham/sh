// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"bytes"
	"fmt"
	"testing"
)

// TestAnEmptyReturnLeavesWhatTheFullReturnLeaves puts an empty shadow back
// both ways — the steps returnAnEmptyShadow takes, and the full return with
// the route off — over every attribute a cell can carry, alone and beside a
// value and each table outside the cell the full return empties, and requires
// the name, its tables and the record to come out the same, with the route
// taken over every attribute and left to the full road wherever a table holds
// the name (#6373).
//
// The route's soundness rests on a fact about the full return: over a record
// saying "none" for every kind, it takes every attribute off. Every bit of
// everyCellAttr is run here, so an attribute added to the cell that a return
// would keep fails this test rather than quietly losing its value at the end
// of every call.
func TestAnEmptyReturnLeavesWhatTheFullReturnLeaves(t *testing.T) {
	type extra struct {
		name  string
		set   func(r *Runner)
		route bool
	}
	extras := []extra{
		{"nothing else", func(*Runner) {}, true},
		{"a value", func(r *Runner) { r.varsPut("n", "v") }, true},
		{"an integer base", func(r *Runner) { r.integerBase = map[string]int{"n": 16} }, false},
		{"a float precision", func(r *Runner) { r.floatPrecision = map[string]int{"n": 2} }, false},
		{"a case listing", func(r *Runner) { r.caseListing = map[string]byte{"n": caseListingRecorded} }, false},
		{"an assigned message", func(r *Runner) { r.assigned = map[string]string{"n": "m"} }, false},
		{"a held export copy", func(r *Runner) { r.envCopyHeld = map[string]heldEnvCopy{"n": {}} }, false},
		{"a reference", func(r *Runner) { r.nameref = map[string]string{"n": "elsewhere"} }, false},
		{"an array", func(r *Runner) { r.arraysPut("n", nil) }, false},
	}
	run := func(on bool, attr cellAttrs, x extra) (string, int) {
		was := emptyShadowShortcut
		emptyShadowShortcut = on
		defer func() { emptyShadowShortcut = was }()
		var out bytes.Buffer
		r := newTestRunner(t, &Runner{Stdout: &out, Stderr: &out})
		sc := r.pushScope(false)
		sc.takeAnEmptyShadow("n")
		x.set(r)
		if attr != 0 {
			r.setAttrs("n", attr, attr)
		}
		r.restoreShadowedName(sc, "n")
		c := r.cell("n")
		state := "no cell"
		if c != nil {
			state = fmt.Sprintf("slots=%b attrs=%b scalar=%q", c.has, c.attrs, c.scalar)
		}
		_, base := r.integerBase["n"]
		_, listing := r.caseListing["n"]
		_, assigned := r.assigned["n"]
		_, copied := r.envCopyHeld["n"]
		_, ref := r.nameref["n"]
		rec := sc.record("n")
		return fmt.Sprintf("%s base=%v listing=%v assigned=%v copy=%v ref=%v live=%d total=%d rec=%b/%b",
			state, base, listing, assigned, copied, ref, r.liveCells, r.attrTotal, rec.has, rec.bits), r.emptyReturns
	}
	taken, want := 0, 0
	for i := range cellAttrCount + 1 {
		var attr cellAttrs
		if i < cellAttrCount {
			attr = 1 << i
		}
		for _, x := range extras {
			full, fullTook := run(false, attr, x)
			short, took := run(true, attr, x)
			if fullTook != 0 {
				t.Fatalf("the full return took the route with it off")
			}
			taken += took
			if x.route {
				want++
			} else if took != 0 {
				t.Errorf("attribute %b, %s: the route took the return; want the full road", attr, x.name)
			}
			if short != full {
				t.Errorf("attribute %b, %s:\nwith the route: %s\nwithout it:     %s", attr, x.name, short, full)
			}
		}
	}
	if taken != want {
		t.Errorf("the route took %d returns; want %d", taken, want)
	}
	if everyCellAttr != 1<<cellAttrCount-1 || bitsSet(everyCellAttr) != cellAttrCount {
		t.Errorf("everyCellAttr is %b, which is not the cell's %d attributes", everyCellAttr, cellAttrCount)
	}
}

// bitsSet counts the attributes in a.
func bitsSet(a cellAttrs) int {
	n := 0
	for ; a != 0; a &= a - 1 {
		n++
	}
	return n
}
