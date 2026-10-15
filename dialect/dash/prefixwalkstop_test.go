// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"strings"
	"testing"
)

// TestTheExternalRouteStopsAtARefusedPrefixEntry pins that a value behind a
// refused prefix entry is never expanded in front of an external command:
// the refusal ends the shell here, so nothing behind it is needed (#6511).
// The values in front of it, and its own, still are.
//
// Measured 2026-10-15 on dash 0.5.12 (`/bin/dash -c` under
// `env -i PATH=/usr/bin:/bin`): `readonly r=1; r=2 B=$(echo s >&2)
// /bin/echo ran` is the refusal alone, and `A=$(echo a >&2) r=$(echo R >&2)
// B=$(echo s >&2) /bin/echo ran` is `a`, `R`, the refusal.
func TestTheExternalRouteStopsAtARefusedPrefixEntry(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"readonly r=1; r=2 B=$(echo s >&2) /bin/echo ran", ""},
		{"readonly r=1; A=$(echo a >&2) r=$(echo R >&2) B=$(echo s >&2) /bin/echo ran", "a\nR\n"},
		{"readonly r=1; r=2 B=$(echo s >&2) nosuchcmd", ""},
	} {
		out, _ := runDash(t, t.TempDir(), tc.src+"\necho after\n")
		if !strings.HasPrefix(out, tc.want) || strings.Contains(out, "s\n") ||
			strings.Contains(out, "ran") || strings.Contains(out, "after") {
			t.Errorf("%s = %q, want %q then the refusal alone", tc.src, out, tc.want)
		}
		if !strings.Contains(out, "r: is read only") {
			t.Errorf("%s = %q, want the refusal", tc.src, out)
		}
	}
}
