// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"runtime"
	"strconv"
	"testing"
)

// TestRecordingAnAutoloadPathDoesNotCopyTheTable pins the cost of fixing one
// more name's file when two thousand are already fixed, which is what a
// startup autoloading a directory of functions by absolute path does (#5873).
// Copying the table out and storing it back made each record cost the whole
// table, so filling it was quadratic.
func TestRecordingAnAutoloadPathDoesNotCopyTheTable(t *testing.T) {
	r := optionStateRunner(t)
	for i := range 2000 {
		autoloadRecordPath(r, "fn"+strconv.Itoa(i), "/fns/fn"+strconv.Itoa(i))
	}
	// Bytes rather than a count: a map is allocated in a few large blocks,
	// so the copy is a handful of allocations and a great many bytes.
	n := 2000
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 100 {
		autoloadRecordPath(r, "fn"+strconv.Itoa(n), "/fns/fn"+strconv.Itoa(n))
		n++
	}
	runtime.ReadMemStats(&after)
	if per := (after.TotalAlloc - before.TotalAlloc) / 100; per > 4096 {
		t.Errorf("recording one path beside two thousand allocated %d bytes; the table is being copied", per)
	}
	for _, i := range []int{0, 1999, n - 1} {
		name := "fn" + strconv.Itoa(i)
		if got, ok := autoloadFixedPath(r, name); !ok || got != "/fns/"+name {
			t.Errorf("%s reads back as %q, %v", name, got, ok)
		}
	}
	if _, ok := autoloadFixedPath(r, "never"); ok {
		t.Error("a name never recorded reads back as fixed")
	}
	autoloadForgetPath(r, "fn7")
	if _, ok := autoloadFixedPath(r, "fn7"); ok {
		t.Error("a forgotten name still reads back as fixed")
	}
}
