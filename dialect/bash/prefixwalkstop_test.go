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

// TestEachFrozenPrefixEntryIsRefusedWhereTheWalkReachesIt pins #6546: bash
// names a second frozen entry after the values in front of it have expanded,
// not alongside the first. Measured 2026-10-15 on bash 5.3.20 and bash 3.2,
// from `-c` and from a script file under `env -i PATH=/usr/bin:/bin`: each row
// is r's refusal, `s`, z's refusal, then what the command writes.
func TestEachFrozenPrefixEntryIsRefusedWhereTheWalkReachesIt(t *testing.T) {
	for _, tc := range []struct{ name, cmd, tail string }{
		{"external", "/bin/echo ran", "ran\nrc=0\n"},
		{"regular builtin", "true", "rc=0\n"},
		{"function", "f", "in\nrc=0\n"},
		{"special builtin", "export q", "rc=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(),
				"readonly r=1 z=1; f(){ echo in; }; r=2 B=$(echo s >&2) z=3 "+tc.cmd+"; echo rc=$?")
			want := "r: readonly variable\ns\n"
			if !strings.Contains(out, want) || !strings.HasSuffix(out, "z: readonly variable\n"+tc.tail) {
				t.Errorf("= %q, want r's refusal, s, z's refusal, then %q", out, tc.tail)
			}
		})
	}
	// A value in front of the first frozen entry still comes out first, and
	// one behind the second comes out after it.
	out, _ := runBash(t, t.TempDir(),
		"readonly r=1 z=1; A=$(echo a >&2) r=2 B=$(echo s >&2) z=3 C=$(echo t >&2) :")
	if !strings.HasPrefix(out, "a\n") || !strings.Contains(out, "r: readonly variable\ns\n") ||
		!strings.HasSuffix(out, "z: readonly variable\nt\n") {
		t.Errorf("= %q, want a, r's refusal, s, z's refusal, t", out)
	}
}
