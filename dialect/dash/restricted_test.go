// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// This shell has no `set -r` at all, and the refusal is what keeps the
// restricted-mode axes out of it.
//
// Measured 2026-09-22 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C`: `set -r` is `set: Illegal option -r` and the shell exits **2**,
// so nothing after it runs and no mode is entered. Named by the `unpinned dash`
// verdicts on Semantics.RestrictedModeIsLeftByTheLetter and its two siblings —
// a pair the graded corpus cannot reach is answered by a Go test that says why
// (#4205).
func TestSetRefusesTheRestrictedLetter(t *testing.T) {
	out, st := runDash(t, t.TempDir(), "set -r\ncd / && echo moved\necho tail\n")
	if !strings.Contains(out, "Illegal option -r") {
		t.Errorf("output %q, want the letter refused", out)
	}
	if st != 2 {
		t.Errorf("status %d, want 2", st)
	}
	// And the refusal ends the input, which is the half that says the mode was
	// never entered rather than entered and then ignored.
	if strings.Contains(out, "moved") || strings.Contains(out, "tail") {
		t.Errorf("output %q, want nothing after the refusal", out)
	}
}
