// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A brace alternative that came to nothing is removed with the word it left empty here — the other side of
// interp.Semantics.BraceEmptyAlternativeIsAField. ksh93u+ and zsh 5.9.2 keep it; 5.3.20 and 3.2.57 agree with each other
// here.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/bash` 5.3.20 and `/bin/bash` 3.2.57, `-c` under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a directory of
// its own; `go version -m` on that path says *not a Go executable*.
//
// The counter prints the field **count**, because `echo` cannot measure this:
// it joins its arguments with a blank, so a row written with it prints the
// same under both readings and could not fail (#4800).
func TestABraceAlternativeThatCameToNothingIsNoFieldHere(t *testing.T) {
	const counter = `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; printf '\n'; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		{"a list of two is the control", `f {a,b}`, "2 | [a] [b]\n"},
		{"a written empty alternative", `f {a,}`, "1 | [a]\n"},
		{"one in front", `f {,a}`, "1 | [a]\n"},
		{"both of them", `f {,}`, "0 |\n"},
		{"one an unset parameter emptied", `unset u; f {a,$u}`, "1 | [a]\n"},
		{"one a substitution emptied", `f {a,$(true)}`, "1 | [a]\n"},
		{"a nested group's", `f {a,{b,}}`, "2 | [a] [b]\n"},
		// Not this question: the alternatives are text, so nothing came to
		// nothing and every column answers two.
		{"a name with text in it", `unset u; f {a,b}$u`, "2 | [a] [b]\n"},
		// Nor this one: a quoted null is a field the word keeps in every
		// column, which is the mechanism the axis extends rather than an
		// exception to it.
		{"a quoted empty alternative", `f {a,""}`, "2 | [a] []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runBash(t, t.TempDir(), counter+tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
