// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

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
// Measured 2026-09-27 against `/opt/homebrew/bin/zsh` 5.9.2 run `-f`, `-c`
// under `env -i PATH=/usr/bin:/bin` with a scratch HOME, each case in a
// directory of its own; `go version -m` on that path says *not a Go
// executable*. ksh93u+ answers every row here identically, so this is not a
// zsh oddity: it is bash that expands the word again for each name.
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
		{"a quoted list", `set -- 1 2; f x{p,q}"$@"y`, "3 | [xp1] [xq1] [2y]\n"},
		{"an array behind the group", `a=(1 2); f x{p,q}${a[@]}y`, "3 | [xp1] [xq1] [2y]\n"},

		// The row that says a field boundary inside the group takes the
		// group away, and the control that holds the shape fixed and varies
		// only whether the expansion in it yields one field or two.
		{"a list inside the group", `set -- 1 2; f x{p,$@}y`, "2 | [x{p,1] [2}y]\n"},
		{"a scalar inside the group", `set -- 1; f x{p,$1}y`, "2 | [xpy] [x1y]\n"},
		{"an empty list joins", `set --; f x{p,q}$@y`, "2 | [xpy] [xqy]\n"},

		// A produced brace, comma or run of them is data here.
		{"a produced group", `e='{a,b}'; f $e`, "1 | [{a,b}]\n"},
		{"a produced group beside written text", `e='{a,b}'; f x$e`, "1 | [x{a,b}]\n"},
		{"a produced opening brace", `e='{'; f $e{a,b}`, "2 | [{a] [{b]\n"},
		{"and a produced pair", `e='{}'; f $e{a,b}`, "2 | [{}a] [{}b]\n"},
		{"a produced comma", `e=a,b; f {$e}`, "1 | [{a,b}]\n"},
		{"produced braces in a list's elements", `a=("x{p" "q}y"); f ${a[@]}`, "2 | [x{p] [q}y]\n"},
		{"a quoted group beside a list", `set -- 1 2; f "x{p,q}"$@y`, "2 | [x{p,q}1] [2y]\n"},

		// The distributive flag composes with it for free, and the **order**
		// is the whole of the tell: the brace varies fastest within a copy
		// of the word, which is what expanding once and brace-expanding each
		// copy afterwards gives. A plan that applied the group at its own
		// position in the word would interleave them the other way round.
		{"the distributive flag", `a=(1 2); f x{p,q}${^a}y`, "4 | [xp1y] [xq1y] [xp2y] [xq2y]\n"},

		// And the splitter still reaches inside a group here, which is the
		// column ksh93 parts from. `shwordsplit` because this shell does not
		// split an unquoted parameter otherwise.
		{"the splitter reaches the body", `setopt shwordsplit; e='a b,c'; f {$e}`, "2 | [{a] [b,c}]\n"},
		{"and behind a brace", `setopt shwordsplit; IFS=:; v=a:b; f x{p,q}$v`, "3 | [xpa] [xqa] [b]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, t.TempDir(), braceCounter+tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
