// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package driver_test

import (
	"runtime/debug"
	"testing"

	"github.com/blairham/sh/driver"
)

// The collector is stopped while the descriptor table is being placed, and
// started again if the exec that was to follow it failed.
//
// Asked here rather than only through a shell, because the shell-level test
// for this can only bite on Linux: the descriptor a placement overwrites there
// is the netpoller's epoll descriptor, and macOS polls with kqueue and numbers
// its descriptors differently — twenty runs out of twenty each way, measured.
// The rule itself is neither platform's, so it is checked where every platform
// can see it.
//
// The exec is made to fail, which is the only way a test can be in the same
// process afterwards: `exec cmd` does not return when it works.
func TestTheCollectorIsStoppedWhileTheTableIsPlaced(t *testing.T) {
	// A value of its own, so that "restored" means restored to what was there
	// rather than to whatever the default happens to be.
	const mine = 137
	was := debug.SetGCPercent(mine)
	t.Cleanup(func() { debug.SetGCPercent(was) })

	// Read on both sides of the placement. Nothing has been overwritten — the
	// table handed over is empty — so asking is safe here, which it would not
	// be in a shell that had just placed a real one.
	//
	// Both halves are asked because they are two different rules. A shell that
	// stopped the collector *after* placing the table would satisfy the second
	// and not the first, and the descriptors would already have been
	// overwritten while a collection could still start.
	seen := map[string]int{}
	driver.AtPlacementForTest(func(where string) {
		seen[where] = debug.SetGCPercent(-1)
	})
	t.Cleanup(func() { driver.AtPlacementForTest(nil) })

	// A path nothing can execute, so the failure path is the one taken.
	err := driver.ReplaceProcessForTest(t.TempDir()+"/not-a-program", []string{"x"}, nil, nil)
	if err == nil {
		t.Fatal("the exec did not fail, so the failure path was never taken")
	}
	for _, where := range []string{"before", "after"} {
		got, ok := seen[where]
		if !ok {
			t.Errorf("the replacement never reached %q", where)
			continue
		}
		if got != -1 {
			t.Errorf("%s the table was placed the collector was at %d, want it stopped (-1)", where, got)
		}
	}
	if back := debug.SetGCPercent(mine); back != mine {
		t.Errorf("after a failed exec the collector is at %d, want the %d it was at before", back, mine)
	}
}
