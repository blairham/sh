// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestTheRunningExitTrapIsListed pins that a bare `trap` inside the running
// EXIT trap lists it. Measured 2026-10-02 on bash 5.3.20 (#5357).
func TestTheRunningExitTrapIsListed(t *testing.T) {
	t.Parallel()
	got, _ := runBash(t, t.TempDir(), "trap 'echo E; trap' EXIT; echo b")
	if want := "b\nE\ntrap -- 'echo E; trap' EXIT\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	// A trap the body sets is the one listed.
	got, _ = runBash(t, t.TempDir(), "trap 'trap \"echo N\" EXIT; trap' EXIT")
	if want := "trap -- 'echo N' EXIT\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
