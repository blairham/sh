// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"testing"

	"github.com/blairham/sh/driver"
)

// nameableCeiling is the highest descriptor this test insists the runtime
// stays above, and it is exactly the ceiling: see lowfds_unix_test.go's
// TestTheRuntimeHoldsNoDescriptorAScriptCanName.
//
// Written out rather than read from the package, and that is the whole point
// of it being here: a test that took the ceiling from the code it is checking
// would follow the ceiling down. Lowering it to 9 — which is all the panel can
// name, and a defensible-looking number — would leave the runtime on 10, 11
// and 12, which is where bash's own `exec {v}>f` allocates from, and every
// assertion in that file would still pass. This one would not.
//
// 56 on Linux rather than 99, because holding past 63 grows the descriptor
// table under an RCU grace period at every start (#6396). See lowfds_linux.go.
const nameableCeiling = 56

// replacementProbes are the numbers the replacement test parks on: every
// single digit, the first number bash's `{v}` allocates, and the ceiling.
var replacementProbes = []int{3, 4, 5, 6, 7, 8, 9, 10, 56}

// kernelFirstTable is how many descriptors a Linux process's table starts
// with, and the first number that makes the kernel grow it.
const kernelFirstTable = 64

// The hold and everything the runtime opens above it stay inside the table the
// process started with.
//
// That is the whole reason the Linux ceiling is 56. Taking descriptor 64 while
// the process has more than one thread waits for an RCU grace period, measured
// at 8 to 13 ms of wall time and 0.3 ms of CPU on every start, so a ceiling
// that drifts up — or a runtime that starts opening a third descriptor — has to
// fail here rather than be found in the performance gate.
//
// The census is what the runtime took, so a runtime descriptor on 64 or above
// is exactly the growth; and the hold reaches only to the ceiling, which the
// last check bounds.
func TestTheHoldNeverGrowsTheDescriptorTable(t *testing.T) {
	held := driver.RuntimeDescriptorsForTest()
	if len(held) == 0 {
		t.Fatal("the runtime opened nothing while the low descriptors were held")
	}
	for _, fd := range held {
		if fd >= kernelFirstTable {
			t.Errorf("the runtime landed on descriptor %d, past the kernel's first %d-entry"+
				" table — every start now waits for the table to grow", fd, kernelFirstTable)
		}
	}
	if c := driver.LowDescriptorCeilingForTest(); c >= kernelFirstTable {
		t.Errorf("the hold reaches %d, past the kernel's first %d-entry table", c, kernelFirstTable)
	}
}
