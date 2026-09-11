// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package acpcheck

import (
	"os/exec"
	"testing"
)

// TestTrueCommandActuallyRuns is the assertion whose absence let the cost
// table measure nothing for as long as it did.
//
// The rows that compare a gated turn against a spawned shell are only a
// comparison if both sides run a program. They named `/bin/true`, which does
// not exist on macOS, and the spawn rows discarded the error — so both sides
// were timing a shell that started, failed to find the program and exited 127,
// and the table reported it as the cost of running a command. The ratio it
// printed was out by most of an order of magnitude.
//
// Checking that the path exists would not be enough: a path can exist and not
// be runnable. This runs it, which is the only claim the cost rows depend on.
func TestTrueCommandActuallyRuns(t *testing.T) {
	prog := TrueCommand()
	// Not a skip. "Found nothing" is precisely the state the old code was in
	// on every Mac in the world, and a test that skips on it would have gone
	// green through the entire life of the bug — which is what the first
	// draft of this test did when the wrong path was put back to check it.
	// Every machine that can run this suite has a true(1); if the lookup
	// cannot find it, the lookup is what is broken.
	if prog == "" {
		t.Fatal("TrueCommand found no true(1): the cost rows have no program to time, and would report a failed lookup as the cost of running a command")
	}
	if err := exec.Command(prog).Run(); err != nil {
		t.Fatalf("%s: the program the cost rows time does not run: %v", prog, err)
	}
}

// TestTrueCommandIsAbsolute keeps the program a program on both sides.
//
// A bare name would be resolved by the shell under test on one side of the
// comparison and by exec.LookPath on the other, and `true` is a builtin in
// every shell here — so one side would time a builtin and the other a process,
// which is the same defect as the one above wearing a different hat.
func TestTrueCommandIsAbsolute(t *testing.T) {
	prog := TrueCommand()
	if prog == "" {
		t.Fatal("TrueCommand found no true(1)")
	}
	if prog[0] != '/' {
		t.Fatalf("TrueCommand returned %q, which a shell would resolve for itself", prog)
	}
}
