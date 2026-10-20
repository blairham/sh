// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The plus listing names what the valued one lists, and says `tagged`**
// (#5157). Measured 2026-10-01 on zsh 5.9.2 under `-f -c`, `env -i
// PATH=/usr/bin:/bin`:
//
//   - `-t` lists as `tagged`, after `readonly` and before `exported`;
//   - `local +` in a function is `typeset +` — names, no values — and `local
//     +x` the exported names;
//   - a bare `typeset +` names the parameters the shell produces, RANDOM and
//     the prompts among them, as a bare `typeset` lists them.
func TestThePlusListing(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"tagged, in its place",
			`typeset -t t1; typeset -rt t2=1; typeset -at t3; typeset -it t4; typeset -Ut t5; typeset -ut t7; typeset -Tt TT tt
f() { local -t l1; local -rxt l2; local -at l3; typeset +m "[tl]?"; typeset +m TT; }; f`,
			"local tagged l1\nlocal readonly tagged exported l2\narray local tagged l3\n" +
				"tagged t1\nreadonly tagged t2\narray tagged t3\ninteger tagged t4\ntagged unique t5\nuppercase tagged t7\n" +
				"array tagged tied TT tt\ntagged tied tt TT\n",
		},
		{
			"local + is typeset +", `f() { local a=1; [[ "$(local +)" == "$(typeset +)" ]] && echo same; local + | /usr/bin/grep -c =; }; f`,
			"same\n0\n",
		},
		{"local +x", `export E=1; f() { local -x l=2; local +x | /usr/bin/grep -E "^(E|l)$"; }; f`, "E\nl\n"},
		{
			"produced names", `typeset + | /usr/bin/grep -E "^(integer 10 )?(RANDOM|SECONDS|USERNAME|PROMPT|array argv|array pipestatus|prompt)$"`,
			"PROMPT\ninteger 10 RANDOM\ninteger 10 SECONDS\nUSERNAME\narray argv\narray pipestatus\nprompt\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
