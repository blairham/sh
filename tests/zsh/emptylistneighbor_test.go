// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The third answer to what an empty `"$@"` takes with it, and the one that
// makes interp.Semantics.EmptyListTakesTheWord three-valued rather than a
// switch: only what stands **before** the list goes with it, so an expansion
// written behind it brings the word back however empty it came out.
//
// Measured 2026-09-22 against `/opt/homebrew/bin/zsh` 5.9.2, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`. bash and ksh93 answer the fourth and
// fifth rows 0 where this answers 1, and dash and BusyBox ash answer every row
// here 1 — a fix written as "bash or not" would have given this column an
// answer no shell has.
func TestOnlyWhatStandsBeforeAnEmptyPositionalListGoesWithIt(t *testing.T) {
	const set = `set --; unset xxx; e=; f=; n(){ echo "$#"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{"the list on its own", `n "$@"`, "0\n"},
		{"an unset name in front of it", `n "$xxx${@}"`, "0\n"},
		{"an empty one in front", `n "$e$@"`, "0\n"},
		{"one behind brings it back", `n "$@$e"`, "1\n"},
		{"and so does one on each side", `n "$e$@$f"`, "1\n"},
		{"a literal brings it back", `n "x$@"`, "1\n"},
		{"the star is one word", `n "$xxx${*}"`, "1\n"},
		// Two lists in one word: the second is not something standing behind
		// the first, so nothing has brought the word back.
		{"two lists in a row", `n "${@}${@}"`, "0\n"},
		{"and something behind the pair", `n "${@}${@}$e"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), set+tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
