// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"strings"
	"testing"
)

// `readonly -n`, which this applet takes and this shell refused as an illegal
// option. What the letter buys a script is the status and nothing else: the
// name is frozen regardless, which is the other reading of a letter one shell
// in the panel uses to *suppress* the freeze — see
// Semantics.ReadonlyReferenceLetter.
//
// Measured 2026-09-18 in the digest-pinned image, BusyBox v1.37.0 (#3464).
func TestReadonlyTakesTheNLetterAndFreezesAnyway(t *testing.T) {
	t.Parallel()
	out, st := run(t, `v=1; readonly -n r=v; echo "a=$?"; echo "r=[$r]"; r=5`)
	if !strings.HasPrefix(out, "a=0\nr=[v]\n") {
		t.Errorf("readonly -n r=v = %q, want the letter taken and the name declared", out)
	}
	if !strings.Contains(out, "read only") || st == 0 {
		t.Errorf("got %q (status %d), want the later write refused", out, st)
	}
}

// The control that says `n` alone was added: the kind letters are still
// refused here, and the refusal ends the script.
func TestReadonlyStillRefusesTheKindLetters(t *testing.T) {
	t.Parallel()
	for _, letter := range []string{"a", "A", "f"} {
		out, st := run(t, `readonly -`+letter+` zz; echo reached`)
		if strings.Contains(out, "reached") || st == 0 {
			t.Errorf("readonly -%s = %q (status %d), want it refused", letter, out, st)
		}
	}
}
