// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// This shell has no `set -r` at all, and the refusal is what keeps the
// restricted-mode axes out of it.
//
// BusyBox v1.37.0 answers `set: line 1: illegal option -r` and exits 2 —
// recorded through the pinned image, since there is no BusyBox on a macOS
// machine. Named by the `unpinned ash` verdicts on
// Semantics.RestrictedModeIsLeftByTheLetter and its two siblings (#4205).
func TestSetRefusesTheRestrictedLetter(t *testing.T) {
	out, st := run(t, "set -r\ncd / && echo moved\necho tail\n")
	if !strings.Contains(out, "illegal option -r") {
		t.Errorf("output %q, want the letter refused", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
	if strings.Contains(out, "moved") || strings.Contains(out, "tail") {
		t.Errorf("output %q, want nothing after the refusal", out)
	}
}
