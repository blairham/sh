// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import (
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

// A shell binary raises the threshold, because a startup is a short
// allocation burst and the default collects through the whole of it. See
// collector.go for the measurement: −29% CPU for +30MB on a real `~/.zshrc`.
func TestAShellBinaryRaisesTheCollectorsThreshold(t *testing.T) {
	restoreGCPercent(t)
	// Named, then taken away, so the cleanup t.Setenv registers still puts
	// back whatever the environment really had.
	t.Setenv("GOGC", "100")
	os.Unsetenv("GOGC")

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
