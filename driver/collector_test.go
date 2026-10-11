// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
	"math"
	"os"
	"runtime/debug"
	"testing"
)

// restoreGCPercent puts the test process's collector back, because this is one
// of the few things a test here really does change about the process it runs
// in — which is the whole reason the call under test is in Main and not in
// MainArgs.
func restoreGCPercent(t *testing.T) {
	t.Helper()
	was := debug.SetGCPercent(startupGCPercent)
	debug.SetGCPercent(was)
	t.Cleanup(func() { debug.SetGCPercent(was) })
}

// restoreMemoryLimit does the same for the ceiling. A negative argument reads
// the limit without changing it.
func restoreMemoryLimit(t *testing.T) {
	t.Helper()
	was := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(was) })
}

// unnamed takes a variable out of the environment for the rest of the test,
// naming it first so the cleanup t.Setenv registers still puts back whatever
// the environment really had.
func unnamed(t *testing.T, name string) {
	t.Helper()
	t.Setenv(name, "x")
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
}

// A shell binary raises the threshold, because a startup is a short
// allocation burst and the default collects through the whole of it. See
// collector.go for the measurement: −29% CPU for +30MB on a real `~/.zshrc`.
func TestAShellBinaryRaisesTheCollectorsThreshold(t *testing.T) {
	restoreGCPercent(t)
	restoreMemoryLimit(t)
	unnamed(t, "GOGC")
	unnamed(t, "GOMEMLIMIT")

	tuneCollector()
	got := debug.SetGCPercent(startupGCPercent)
	if got != startupGCPercent {
		t.Errorf("threshold is %d, want %d", got, startupGCPercent)
	}
}

// And an explicit GOGC is left exactly as it was. The runtime has already
// applied it, and the person who set it knows more about their machine than a
// constant here does — including having set it to `off`, which is a negative
// number and which this must not quietly undo.
func TestAnExplicitGOGCIsNotOverridden(t *testing.T) {
	restoreGCPercent(t)
	for _, named := range []string{"100", "off", "50"} {
		t.Run(named, func(t *testing.T) {
			t.Setenv("GOGC", named)
			// Whatever the process is at now is what must survive the call.
			was := debug.SetGCPercent(startupGCPercent)
			debug.SetGCPercent(was)

			tuneCollector()
			if got := debug.SetGCPercent(was); got != was {
				t.Errorf("GOGC=%s: threshold moved to %d, want %d left alone", named, got, was)
			}
		})
	}
}

// And a ceiling beside it, because a percent has none: at 1600 a heap may grow
// to seventeen times what it keeps. See collector.go for the measurement that
// set the value — null on an interactive start, and a runaway script held at
// it rather than at hundreds of megabytes more.
func TestAShellBinaryPutsACeilingBesideThePercent(t *testing.T) {
	restoreGCPercent(t)
	restoreMemoryLimit(t)
	unnamed(t, "GOGC")
	unnamed(t, "GOMEMLIMIT")
	debug.SetMemoryLimit(math.MaxInt64)

	tuneCollector()
	if got := debug.SetMemoryLimit(-1); got != runawayHeapLimit {
		t.Errorf("memory limit is %d, want %d", got, int64(runawayHeapLimit))
	}
}

// An explicit GOMEMLIMIT is left as the runtime applied it, just as an
// explicit GOGC is, and the percent is still raised: naming the ceiling says
// nothing about the threshold under it.
func TestAnExplicitGOMEMLIMITIsNotOverridden(t *testing.T) {
	restoreGCPercent(t)
	restoreMemoryLimit(t)
	unnamed(t, "GOGC")
	for _, named := range []string{"1GiB", "off", "64MiB"} {
		t.Run(named, func(t *testing.T) {
			t.Setenv("GOMEMLIMIT", named)
			// A value of the test's own stands in for whatever the runtime
			// applied, so "left alone" cannot pass by coinciding with the
			// constant.
			const mine = 123 << 20
			debug.SetMemoryLimit(mine)

			tuneCollector()
			if got := debug.SetMemoryLimit(-1); got != mine {
				t.Errorf("GOMEMLIMIT=%s: limit moved to %d, want %d left alone", named, got, int64(mine))
			}
			if got := debug.SetGCPercent(startupGCPercent); got != startupGCPercent {
				t.Errorf("GOMEMLIMIT=%s: threshold is %d, want %d", named, got, startupGCPercent)
			}
		})
	}
}

// A GOGC alone leaves the ceiling unset too. The ceiling belongs to the
// shipped percent, and someone who chose their own — `off` above all — would
// otherwise find their collector running at a limit they never asked for.
func TestAnExplicitGOGCLeavesTheCeilingUnset(t *testing.T) {
	restoreGCPercent(t)
	restoreMemoryLimit(t)
	unnamed(t, "GOMEMLIMIT")
	for _, named := range []string{"100", "off"} {
		t.Run(named, func(t *testing.T) {
			t.Setenv("GOGC", named)
			debug.SetMemoryLimit(math.MaxInt64)

			tuneCollector()
			if got := debug.SetMemoryLimit(-1); got != math.MaxInt64 {
				t.Errorf("GOGC=%s: limit set to %d, want none", named, got)
			}
		})
	}
}
