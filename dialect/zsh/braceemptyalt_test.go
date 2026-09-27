// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A brace alternative that came to nothing is a field of its own here — the other side of
// interp.Semantics.BraceEmptyAlternativeIsAField. ksh93u+ answers every row here the same way, so this is not a zsh
// oddity; #4800 filed the row as ksh93's alone and this column was never
// asked.
//
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` 5.9.2, run `-f`, `-c` under
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a directory of
// its own; `go version -m` on that path says *not a Go executable*.
//
// The counter prints the field **count**, because `echo` cannot measure this:
// it joins its arguments with a blank, so a row written with it prints the
// same under both readings and could not fail (#4800).
func TestABraceAlternativeThatCameToNothingIsAFieldHere(t *testing.T) {
	const counter = `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; printf '\n'; }` + "\n"
	for _, tc := range []struct{ name, src, want string }{
		{"a list of two is the control", `f {a,b}`, "2 | [a] [b]\n"},
		{"a written empty alternative", `f {a,}`, "2 | [a] []\n"},
		{"one in front", `f {,a}`, "2 | [] [a]\n"},
		{"both of them", `f {,}`, "2 | [] []\n"},
		{"one an unset parameter emptied", `unset u; f {a,$u}`, "2 | [a] []\n"},
		{"one a substitution emptied", `f {a,$(true)}`, "2 | [a] []\n"},
		{"a nested group's", `f {a,{b,}}`, "3 | [a] [b] []\n"},
		// Not this question: the alternatives are text, so nothing came to
		// nothing and every column answers two.
		{"a name with text in it", `unset u; f {a,b}$u`, "2 | [a] [b]\n"},
		// Nor this one: a quoted null is a field the word keeps in every
		// column, which is the mechanism the axis extends rather than an
		// exception to it.
		{"a quoted empty alternative", `f {a,""}`, "2 | [a] []\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, t.TempDir(), counter+tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
