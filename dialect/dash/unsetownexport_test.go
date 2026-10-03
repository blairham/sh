// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// TestUnsetKeepsTheExportOfTheShellsOwnParameter pins that `unset` on
// `LINENO`, a parameter the shell owns, leaves its export letter, so the
// value assigned next reaches a child; a name the script made loses it.
// Measured 2026-10-03 on dash 0.5.12 under `env -i`. See
// Semantics.UnsetKeepsTheShellsOwnAttributes.
func TestUnsetKeepsTheExportOfTheShellsOwnParameter(t *testing.T) {
	const env = "/usr/bin/env | while read -r kv; do case $kv in LINENO=*|FOO=*) echo \"$kv\";; esac; done\n"
	for _, tc := range []struct{ src, want string }{
		{"export LINENO=5; unset LINENO; LINENO=7\n" + env, "LINENO=7\n"},
		{"export FOO=1; unset FOO; FOO=2\n" + env, ""},
	} {
		got, _ := runDash(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
