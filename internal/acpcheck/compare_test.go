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
	if prog == "" {
		t.Skip("no true(1) on this machine; the cost rows skip too, rather than timing a failure")
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
		t.Skip("no true(1) on this machine")
	}
	if prog[0] != '/' {
		t.Fatalf("TrueCommand returned %q, which a shell would resolve for itself", prog)
	}
}
