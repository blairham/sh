// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"
)

// An empty expansion written beside a `"$@"` that has no positional parameters
// does not bring the word back here, on either side — see
// interp.Semantics.EmptyListTakesTheWord, where bash and ksh93 are one of the
// three answers and zsh and dash are the others.
//
// Measured 2026-09-22 against `/opt/homebrew/bin/bash` 5.3.20, script files
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`, with `set --; unset xxx; e=; f=`
// and `n(){ echo "$#"; }`.
func TestAnEmptyExpansionBesideAnEmptyPositionalListTakesTheWord(t *testing.T) {
	const set = `set --; unset xxx; e=; f=; n(){ echo "$#"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{"the list on its own", `n "$@"`, "0\n"},
		{"an unset name in front of it", `n "$xxx${@}"`, "0\n"},
		{"an empty one in front", `n "$e$@"`, "0\n"},
		{"and one behind", `n "$@$e"`, "0\n"},
		{"one on each side", `n "$e$@$f"`, "0\n"},
		{"and a braced list between them", `n "$e${@}$f"`, "0\n"},
		// The boundary, and the two rows that say this is about `$@` and about
		// expansions that came out empty rather than about concatenation.
		{"a literal brings it back", `n "x$@"`, "1\n"},
		{"and so does a value", `q=QQ; n "$q$@"`, "1\n"},
		{"the star is one word", `n "$xxx${*}"`, "1\n"},
		// The reading is about one quoted string: a pair of quotes of its own
		// is a quoted null the word keeps.
		{"a string of its own in front", `n "$e""$@"`, "1\n"},
		{"and one behind", `n "$@""$e"`, "1\n"},
		{"a quoted null literal", `n "$@"''`, "1\n"},
		// Unquoted, where an expansion that came to nothing is no field in
		// every column anyway.
		{"an unquoted neighbor", `n $e$@`, "0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), set+tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// And an empty array's `[@]` is the same question, measured the same day: with
// `a=()`, `"$e${a[@]}"` and `"${a[@]}$e"` are both no argument at all where
// `"x${a[@]}"` is one.
func TestAnEmptyArrayBesideAnEmptyExpansionTakesTheWord(t *testing.T) {
	const set = `a=(); e=; n(){ echo "$#"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{"in front of it", `n "$e${a[@]}"`, "0\n"},
		{"and behind", `n "${a[@]}$e"`, "0\n"},
		{"the array on its own", `n "${a[@]}"`, "0\n"},
		{"a literal brings it back", `n "x${a[@]}"`, "1\n"},
		{"and the star is one word", `n "$e${a[*]}"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), set+tc.src); out != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}
