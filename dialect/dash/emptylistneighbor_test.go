// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"
)

// An empty `"$@"` takes nothing with it here: a quoted expansion written beside
// it is a field whether or not it produced anything. The third of the three
// answers interp.Semantics.EmptyListTakesTheWord has, and the one this shell
// used to give in every column.
//
// Measured 2026-09-22 against `/bin/dash` 0.5.12, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`. BusyBox ash answers the same way.
func TestAnEmptyExpansionBesideAnEmptyPositionalListKeepsTheWord(t *testing.T) {
	const set = `set --; e=; n(){ echo "$#"; }; `
	for _, tc := range []struct{ name, src, want string }{
		// Still unanimous: the list on its own is no word at all.
		{"the list on its own", `n "$@"`, "0\n"},
		{"an empty one in front", `n "$e$@"`, "1\n"},
		{"and one behind", `n "$@$e"`, "1\n"},
		{"a literal brings it back too", `n "x$@"`, "1\n"},
		{"an unquoted neighbor does not", `n $e$@`, "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runDash(t, t.TempDir(), set+tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
