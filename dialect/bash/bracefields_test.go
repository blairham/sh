// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// braceCounter prints the field **count** and then the fields, because `echo`
// cannot measure any of this: it joins its arguments with a blank (#4561).
const braceCounter = `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; printf '\n'; }` + "\n"

// This shell finds the braces in the word the parse cut and expands the word
// again for every name they make — the side of
// interp.Semantics.BraceFanExpandsEachNameOnItsOwn that ksh93 and zsh do not
// take, and the reason a list in the word is copied once per name.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash`
// 3.2.57, which agree on every row; `-c` under `env -i PATH=/usr/bin:/bin`
// with a scratch HOME, each case in a directory of its own. `go version -m`
// says *not a Go executable* for both.
func TestTheBracesAreFoundInTheWordHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a group and nothing else", `set -- 1 2; f x{p,q}y`, "2 | [xpy] [xqy]\n"},
		{"a list behind the group", `set -- 1 2; f x{p,q}$@y`, "4 | [xp1] [2y] [xq1] [2y]\n"},
		{"with nothing behind it", `set -- 1 2; f {p,q}$@`, "4 | [p1] [2] [q1] [2]\n"},
		{"a list in front of it", `set -- 1 2; f $@{p,q}y`, "4 | [1] [2py] [1] [2qy]\n"},
		{"a list inside the group", `set -- 1 2; f x{p,$@}y`, "3 | [xpy] [x1] [2y]\n"},
		{"a scalar inside the group", `set -- 1; f x{p,$1}y`, "2 | [xpy] [x1y]\n"},
		{"an empty list joins", `set --; f x{p,q}$@y`, "2 | [xpy] [xqy]\n"},
		{"a brace stops no splitter", `IFS=:; v=a:b; f x{p,q}$v`, "4 | [xpa] [b] [xqa] [b]\n"},
		{"a produced group is data", `e='{a,b}'; f $e`, "1 | [{a,b}]\n"},
		// And what the braces produced goes back into the word as shell
		// **text** here, so the two names are `$ea` and `$eb`, which are
		// unset: this row is that axis and this one together.
		{"and so is a produced opening brace", `e='{'; f $e{a,b}`, "0 |\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runBash(t, t.TempDir(), braceCounter+tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
