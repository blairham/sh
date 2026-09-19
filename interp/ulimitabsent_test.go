// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strconv"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A row's value has three shapes, not two, and the third is what
// UlimitListingRow.Absent is for: a row that reads this kernel's number where
// there is one and prints a sentence of the engine's own where there is not.
//
// TestAnAbsentRowIsALimitOnOneKernelAndASentenceOnAnother pins all four
// quadrants — read and set, limit present and missing — because the value and
// the refusal move independently and a table that got one right and the other
// wrong would still print a plausible page.

func absentTable() Diagnostics {
	return Diagnostics{
		UlimitListing: []UlimitListingRow{
			{Prefix: "cpu time  ", Letter: 't', Res: ResourceCPUTime, Scale: 1},
			{Prefix: "locks     ", Letter: 'x', Res: ResourceFileLocks, Scale: 1, Absent: "not supported", Name: "locks"},
		},
		UlimitReadOnly: "ulimit: %[1]s: is read only",
	}
}

func TestAnAbsentRowIsALimitOnOneKernelAndASentenceOnAnother(t *testing.T) {
	everything := func(Resource) bool { return true }
	without := func(res Resource) bool { return res != ResourceFileLocks }

	// A kernel that has the limit: an ordinary live row, read and written.
	held := limits(map[Resource]limitPair{ResourceFileLocks: {77, 77}, ResourceCPUTime: {60, 60}})
	if out, st := ulimitRunTable(t, held, absentTable(), everything, nil, `ulimit -x`); st != 0 || strings.TrimSpace(out) != "77" {
		t.Errorf("-x where the kernel has the limit: %q status %d, want 77 at 0", out, st)
	}
	if out, st := ulimitRunTable(t, held, absentTable(), everything, nil, `ulimit -x 100`); st != 0 || out != "" {
		t.Errorf("setting -x where the kernel has the limit: %q status %d, want silence at 0", out, st)
	}
	if got := held[ResourceFileLocks]; got.soft != 100 {
		t.Errorf("the limit is now %d, want the 100 that was set", got.soft)
	}
	if out, st := ulimitRunTable(t, held, absentTable(), everything, nil, `ulimit -a`); st != 0 || out != "cpu time  60\nlocks     100\n" {
		t.Errorf("`ulimit -a` where the kernel has the limit: %q status %d", out, st)
	}

	// And a kernel that does not: the sentence is the whole value, the
	// letter is still read, and a set is refused as read only.
	held = limits(map[Resource]limitPair{ResourceCPUTime: {60, 60}})
	if out, st := ulimitRunTable(t, held, absentTable(), without, nil, `ulimit -x`); st != 0 || strings.TrimSpace(out) != "not supported" {
		t.Errorf("-x where the kernel has no such limit: %q status %d, want the sentence at 0", out, st)
	}
	out, st := ulimitRunTable(t, held, absentTable(), without, nil, `ulimit -x 100`)
	if st != 1 || !strings.Contains(out, "ulimit: locks: is read only") {
		t.Errorf("setting -x where the kernel has no such limit: %q status %d, want the read-only refusal at 1", out, st)
	}
	if _, set := held[ResourceFileLocks]; set {
		t.Error("the refused set reached the kernel anyway")
	}
	// The row stays in the table, which is the difference from a plain live
	// row: that one leaves and takes its letter with it (#2805).
	if out, st := ulimitRunTable(t, held, absentTable(), without, nil, `ulimit -a`); st != 0 || out != "cpu time  60\nlocks     not supported\n" {
		t.Errorf("`ulimit -a` where the kernel has no such limit: %q status %d, want the row kept", out, st)
	}
}

// TestAReadOnlyRowIsReadFromTheKernelAndStillRefusesToBeSet is the other half:
// a row whose number is the platform's own — a pipe buffer rather than a limit
// — moves with the kernel while the refusal does not. Neither Fixed nor Absent
// can say that: the first holds one kernel's number as text, and the second
// speaks only where the number is missing.
func TestAReadOnlyRowIsReadFromTheKernelAndStillRefusesToBeSet(t *testing.T) {
	dg := func() Diagnostics {
		return Diagnostics{
			UlimitListing: []UlimitListingRow{
				{Prefix: "pipe(bytes) ", Letter: 'p', Res: ResourcePipeBuffer, Scale: 1, ReadOnly: true, Name: "pipe"},
			},
			UlimitReadOnly: "ulimit: %[1]s: is read only",
		}
	}
	everything := func(Resource) bool { return true }
	for _, size := range []int64{512, 4096} {
		held := limits(map[Resource]limitPair{ResourcePipeBuffer: {size, size}})
		out, st := ulimitRunTable(t, held, dg(), everything, nil, `ulimit -a`)
		if want := "pipe(bytes) " + strconv.FormatInt(size, 10) + "\n"; st != 0 || out != want {
			t.Errorf("`ulimit -a` at a %d-byte buffer: %q status %d, want %q at 0", size, out, st, want)
		}
		out, st = ulimitRunTable(t, held, dg(), everything, nil, `ulimit -p 100`)
		if st != 1 || !strings.Contains(out, "ulimit: pipe: is read only") {
			t.Errorf("setting it: %q status %d, want the read-only refusal at 1", out, st)
		}
		if got := held[ResourcePipeBuffer]; got.soft != size {
			t.Errorf("the refused set moved the value to %d", got.soft)
		}
	}
}
