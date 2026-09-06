// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package driver_test

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// prSetDumpable and prGetDumpable are prctl's. Written out rather than taken
// from a dependency: they are two numbers, fixed by the kernel's ABI, and
// these tests have no other reason to reach outside the standard library.
const (
	prSetDumpable = 4
	prGetDumpable = 3
)

// refuseToBeDumped tells the kernel this process is not to be dumped, which is
// the half of writeNoCore that a piped core_pattern does not ignore.
//
// The resource limit is consulted only for a dump the kernel writes itself, so
// a machine whose core_pattern names a crash reporter dumps anyway — measured
// at 156ms per death against 1ms, on a `--cpus=1` container with a
// race-instrumented binary and nothing else running. The flag has no such
// exception: there is no image to write and no helper to start.
//
// Ignoring the error is the right answer rather than laziness. This is a guard
// against a slow death, not a requirement for a correct one: a kernel that
// refuses it leaves these tests measuring what they measured before, which is
// the state this improves on rather than a regression from it.
func refuseToBeDumped() {
	_, _, _ = syscall.Syscall(syscall.SYS_PRCTL, prSetDumpable, 0, 0)
}

// dumpable is what the kernel currently says about this process.
func dumpable() int {
	n, _, _ := syscall.Syscall(syscall.SYS_PRCTL, prGetDumpable, 0, 0)
	return int(n)
}

// notDumpableProbe names the environment entry that turns this test binary
// into the probe below, the way dyingScript turns it into a shell.
const notDumpableProbe = "SH_TEST_NOT_DUMPABLE_PROBE"

// TestWriteNoCoreLeavesTheProcessUndumpable asserts the guard is *in place*
// rather than asserting how fast a death was, which is the whole point of
// #1088: the cost of a missing guard is a slower death on a loaded machine,
// and a test that measured the slowness would be the flaky assertion this
// change removes.
//
// In a child process, because the flag is process-wide and lasts: setting it
// on the test binary would change what /proc says about it for every test
// after this one, which is a side effect a guard against core dumps has no
// business having.
func TestWriteNoCoreLeavesTheProcessUndumpable(t *testing.T) {
	if os.Getenv(notDumpableProbe) != "" {
		writeNoCore()
		// Through standard output, read by the parent below. os.Exit rather
		// than returning, so the testing framework does not print a summary
		// line after it.
		_, _ = os.Stdout.WriteString("dumpable=" + string(rune('0'+dumpable())) + "\n")
		os.Exit(0)
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestWriteNoCoreLeavesTheProcessUndumpable")
	cmd.Env = append(os.Environ(), notDumpableProbe+"=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the probe half failed: %v: %s", err, out)
	}
	if got := strings.TrimSpace(string(out)); got != "dumpable=0" {
		t.Errorf("after writeNoCore the child says %q, want dumpable=0 — "+
			"the resource limit alone is ignored where core_pattern pipes to a "+
			"crash reporter, which is what made a death take four seconds", got)
	}
}
