// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// TestTheExternalRouteStopsAtARefusedPrefixEntry pins that a value behind a
// refused prefix entry is never expanded in front of an external command,
// while the values in front of it and its own value are: ksh93 gives the
// command up at the refusal, so nothing behind it is needed (#6511). And
// that an array literal's elements are expanded as a prefix (#6512).
//
// Measured 2026-10-15 on ksh93u+ (`/bin/ksh -c` under
// `env -i PATH=/usr/bin:/bin`):
//
//	readonly r=1; r=2 B=$(echo s >&2) /bin/echo ran                     the refusal, rc=1
//	readonly r=1; A=$(echo a >&2) r=$(echo R >&2) B=$(echo s >&2) …    a, R, the refusal
//	readonly r=1; r=($(echo s >&2)) /bin/echo ran                       s, the refusal
//	x=($(echo s >&2) b) /bin/echo ran                                   s, ran
func TestTheExternalRouteStopsAtARefusedPrefixEntry(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a value behind it", "readonly r=1; r=2 B=$(echo s >&2) /bin/echo ran; echo rc=$?", "rc=1"},
		{"values on both sides", "readonly r=1; A=$(echo a >&2) r=$(echo R >&2) B=$(echo s >&2) /bin/echo ran; echo rc=$?", "a\nR\n"},
		{"a name nothing runs", "readonly r=1; r=2 B=$(echo s >&2) nosuchcmd; echo rc=$?", "rc=1"},
		{"a literal to a readonly name", "readonly r=1; r=($(echo s >&2)) /bin/echo ran; echo rc=$?", "s\n"},
		{"an ordinary literal", "x=($(echo s >&2) b) /bin/echo ran", "s\nran\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), tc.src)
			if !strings.Contains(out, tc.want) {
				t.Errorf("%s = %q, want it to hold %q", tc.src, out, tc.want)
			}
			if strings.Contains(tc.src, "r=2") || strings.Contains(tc.src, "r=$(") {
				if strings.Contains(out, "s\n") || strings.Contains(out, "ran") {
					t.Errorf("%s = %q, want nothing behind the refused entry expanded and the command unrun", tc.src, out)
				}
			}
		})
	}
}
