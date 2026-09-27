// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// braceCounter prints the field **count** and then the fields, because `echo`
// cannot measure any of this: it joins its arguments with a blank, so
// `[xp1] [xq1] [2y]` and `[xp1] [2y] [xq1] [2y]` differ only in a count `echo`
// never shows (#4561).
const braceCounter = `f(){ printf '%d |' $#; for x in "$@"; do printf ' [%s]' "$x"; done; printf '\n'; }` + "\n"

// This shell expands the word **once**, with its braces as inert text, and
// finds the braces in the fields that came out — the other side of
// interp.Semantics.BraceFanExpandsEachNameOnItsOwn.
//
// Measured 2026-09-27 against `/bin/ksh` `Version AJM 93u+ 2012-08-01`, `-c`
// under `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a
// directory of its own; `go version -m` on that path says *not a Go
// executable*. zsh 5.9.2 answers the first block identically; the splitting
// block is this column's alone.
func TestTheBracesAreFoundInTheFieldsHere(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a group and nothing else", `set -- 1 2; f x{p,q}y`, "2 | [xpy] [xqy]\n"},
		{"a list behind the group", `set -- 1 2; f x{p,q}$@y`, "3 | [xp1] [xq1] [2y]\n"},
		{"with nothing behind it", `set -- 1 2; f {p,q}$@`, "3 | [p1] [q1] [2]\n"},
		{"a list in front of it", `set -- 1 2; f $@{p,q}y`, "3 | [1] [2py] [2qy]\n"},
		{"a list on both sides", `set -- 1 2; f $@{p,q}$@`, "4 | [1] [2p1] [2q1] [2]\n"},
		{
			"two groups and two lists", `set -- 1 2; f x{p,q}$@y{r,s}$@z`,
			"5 | [xp1] [xq1] [2yr1] [2ys1] [2z]\n",
		},
		{
			"three parameters and two groups", `set -- 1 2 3; f {p,q}$@{r,s}`,
			"5 | [p1] [q1] [2] [3r] [3s]\n",
		},
		{"a quoted list", `set -- 1 2; f x{p,q}"$@"y`, "3 | [xp1] [xq1] [2y]\n"},

		{"a list inside the group", `set -- 1 2; f x{p,$@}y`, "2 | [x{p,1] [2}y]\n"},
		{"a scalar inside the group", `set -- 1; f x{p,$1}y`, "2 | [xpy] [x1y]\n"},
		{"an empty list joins", `set --; f x{p,q}$@y`, "2 | [xpy] [xqy]\n"},

		// The body reading, which survives the move: on this road it is the
		// splitter stopping at the brace that leaves the body in one field,
		// and the produced comma being read in it that makes it a list.
		{"a produced comma", `e=a,b; f {$e}`, "2 | [a] [b]\n"},
		{
			"a produced comma beside written text", `e=a,b; f pre{$e}post`,
			"2 | [preapost] [prebpost]\n",
		},
		{"a body with a blank in it", `e='a b,c'; f {$e}`, "2 | [a b] [c]\n"},
		{"a produced alternative is inert", `e='a*,z'; f {$e}`, "2 | [a*] [z]\n"},
		{"and is not read as shell text", `x=BOOM; e='$x,b'; f {$e}`, "2 | [$x] [b]\n"},
		{"a quoted expansion in the body still counts", `e=a,b; f {"$e"}`, "2 | [a] [b]\n"},
		{"a quoted group does not", `e=a,b; f "{$e}"`, "1 | [{a,b}]\n"},
		{"a produced range", `e=1..3; f {$e}`, "3 | [1] [2] [3]\n"},
		{
			"a produced opening brace takes the word", `e='{'; f {c,d$e}{a,b}`,
			"1 | [{c,d{}{a,b}]\n",
		},
		{"a produced closing brace is data", `e='}'; f {a,b$e,c}`, "3 | [a] [b}] [c]\n"},

		// A written brace ends field splitting for the rest of the word,
		// which is this column's alone.
		{"a brace stops the splitter", `IFS=:; v=a:b; f x{p,q}$v`, "2 | [xpa:b] [xqa:b]\n"},
		{"even one that is no list", `IFS=:; v=a:b; f x{p}$v`, "1 | [x{p}a:b]\n"},
		{
			"what is in front of it still splits", `IFS=:; v=a:b; w=c:d; f $v{p,q}$w`,
			"3 | [a] [bpc:d] [bqc:d]\n",
		},
		{"a quoted brace stops nothing", `IFS=:; v=a:b; f "x{"$v`, "2 | [x{a] [b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runKsh(t, t.TempDir(), braceCounter+tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
