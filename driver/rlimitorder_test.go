// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestRlimitOrderIsTheKernelsNumbering: the order one table is listed in is
// this kernel's own numbering, one entry per number — see Runner.RlimitOrder,
// and #2806 for the shell that lists that way.
//
// Asserted against the table the platform files fill in rather than against a
// list written twice, so it holds on either machine: every entry is a limit
// this build has, the numbers rise, and no number appears twice.
func TestRlimitOrderIsTheKernelsNumbering(t *testing.T) {
	order := rlimitOrder()
	if len(order) == 0 {
		t.Fatal("no limits at all — every platform this builds for has some")
	}
	seen := map[int]bool{}
	last := -1
	for i, res := range order {
		id, ok := rlimitOf[res]
		if !ok {
			t.Fatalf("entry %d names a limit this build does not have", i)
		}
		if seen[id] {
			t.Errorf("number %d appears twice: the order is one row per number", id)
		}
		if id <= last {
			t.Errorf("entry %d is number %d, behind %d — the order is the kernel's", i, id, last)
		}
		seen[id], last = true, id
	}
	// And where one number carries two names, the entry is the address
	// space: the shell that lists in this order prints that row for it and
	// has no resident-set row there at all.
	if id, ok := rlimitOf[interp.ResourceAddressSpace]; ok {
		var found bool
		for _, res := range order {
			if res == interp.ResourceAddressSpace {
				found = true
			}
		}
		if !found {
			t.Errorf("the address space is numbered %d here and is not in the order", id)
		}
	}
}
