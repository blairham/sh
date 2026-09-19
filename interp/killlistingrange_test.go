// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"testing"
)

// The bare listing covers every position the kernel will take, not every
// position the table has a name for (#3792).
//
// In package rather than beside the other listing tests because the two
// questions the walk sits between — what this shell can name, and how far
// `kill(2)` goes — are answered by a table and a constant that the build
// picks, and only one of the two platforms has them disagreeing. On the one
// where they do, the gap is thirty-three positions wide and the listing said
// every one of them did not exist while `kill -l N` answered for it; on the
// other there is no gap at all, so a test that could only go through
// platformSignalMax passes there whichever walk is underneath it. Handing the
// bound in is what makes this the same assertion on both.
//
// The rendering stays the dialect's throughout: filling a position is what
// KillListingUnnamedPosition means, and the empty value is a third answer
// rather than a missing one — which is the control at the bottom, and without
// it a walk that filled every gap regardless of the dialect would pass.
func TestTheListingFillsEveryPositionTheKernelTakes(t *testing.T) {
	last := 0
	for _, k := range knownSignals {
		last = max(last, int(k.Sig))
	}
	named := map[int]string{}
	for _, k := range knownSignals {
		named[int(k.Sig)] = k.Name
	}
	// Far enough past the last name to stand for the real-time range without
	// standing for its size, which is one kernel's number and not a rule.
	bound := last + 9

	dg := Diagnostics{KillListingUnnamedPosition: "%[1]d"}
	cells := newTestRunner(t, &Runner{Diagnostics: &dg}).signalListingUpTo(bound)
	at := map[int]string{}
	for _, c := range cells {
		if _, dup := at[c.sig]; dup {
			t.Errorf("position %d written twice", c.sig)
		}
		at[c.sig] = c.text
	}
	for n := 1; n <= bound; n++ {
		text, ok := at[n]
		if !ok {
			t.Errorf("position %d is missing from the listing, and the kernel takes it", n)
			continue
		}
		// A position the table names keeps its name; one past the table gets
		// the dialect's rendering of a position it cannot name. Without this
		// half a walk that numbered every row would pass the one above.
		want := named[n]
		if want == "" {
			want = strconv.Itoa(n)
		}
		if text != want {
			t.Errorf("position %d wrote %q, want %q", n, text, want)
		}
	}
	if len(at) != bound {
		t.Errorf("the listing wrote %d positions for a bound of %d", len(at), bound)
	}

	// The control. A dialect that leaves a position it cannot name out leaves
	// out the ones past the table too, so the fix adds rows to the two
	// dialects that carry a rendering and to no others.
	empty := Diagnostics{}
	for _, c := range newTestRunner(t, &Runner{Diagnostics: &empty}).signalListingUpTo(bound) {
		if c.sig > last {
			t.Errorf("position %d was written as %q by a dialect that renders nothing for a position it cannot name", c.sig, c.text)
		}
	}
}
