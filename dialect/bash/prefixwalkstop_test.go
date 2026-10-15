// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// TestTheExternalRouteGoesOnPastARefusedPrefixEntry is the control for the
// columns that stop at a refused entry (#6511): bash reports the refusal and
// runs the command anyway, so the values behind the refused entry are still
// expanded and the child is still started. Measured 2026-10-15 on bash 5.3.20
// (`-c` under `env -i PATH=/usr/bin:/bin`): `readonly r=1; r=2 B=$(echo s
// >&2) /bin/echo ran; echo rc=$?` is the refusal, `s`, `ran`, `rc=0`, and
// `readonly r=1; r=2 B=hi /usr/bin/printenv B` hands the child `B`.
func TestTheExternalRouteGoesOnPastARefusedPrefixEntry(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), "readonly r=1; r=2 B=$(echo s >&2) /bin/echo ran; echo rc=$?")
	if !strings.Contains(out, "r: readonly variable\ns\nran\nrc=0\n") {
		t.Errorf("= %q, want the refusal, then s, ran and rc=0", out)
	}
	// The entry behind the refused one still reaches the child.
	out, _ = runBash(t, t.TempDir(), "readonly r=1; r=2 B=hi /usr/bin/printenv B; echo rc=$?")
	if !strings.HasSuffix(out, "hi\nrc=0\n") {
		t.Errorf("= %q, want the child handed B=hi and rc=0", out)
	}
}
