// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// TestTheRunningExitTrapIsNotListed pins that a bare `trap` inside the
// running EXIT trap lists nothing here. Measured 2026-10-02 on dash (#5357).
func TestTheRunningExitTrapIsNotListed(t *testing.T) {
	got, _ := runDash(t, t.TempDir(), "trap 'echo E; trap' EXIT; echo b")
	if want := "b\nE\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
