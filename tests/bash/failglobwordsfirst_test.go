// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Under `failglob` every word of a list is expanded before a pattern that
// matched nothing is refused, so a substitution after the pattern still runs
// (#6677). Measured 2026-10-15 on bash 5.3.20 with -c and from a file, in a
// directory where nothing matches; every `want` is that shell's output. See
// interp.Runner.holdPathnameRefusal.
func TestFailglobRefusesAfterEveryWordIsExpanded(t *testing.T) {
	const on = "shopt -s failglob; "
	for _, tc := range []struct{ name, src, want string }{
		{
			"a command's words",
			"echo $(echo S >&2) zz*zz $(echo A >&2); echo st=$?",
			"S\nA\nbash: line 1: no match: zz*zz\n",
		},
		{
			"the command's own name",
			"zz*zz $(echo A >&2); echo st=$?",
			"A\nbash: line 1: no match: zz*zz\n",
		},
		{
			"an array literal",
			"a=(5 $(echo S >&2) zz*zz $(echo A >&2)); echo st=$?",
			"S\nA\nbash: line 1: no match: zz*zz\n",
		},
		{
			"a for list",
			"for i in zz*zz $(echo A >&2); do echo in; done; echo st=$?",
			"A\nbash: line 1: no match: zz*zz\n",
		},
		{
			"only the first pattern is named",
			"echo zz*zz yy*yy $(echo A >&2); echo st=$?",
			"A\nbash: line 1: no match: zz*zz\n",
		},
		{
			// The redirection is not expanded after the refusal.
			"a redirection target",
			"echo zz*zz $(echo A >&2) >$(echo R >&2)f; echo st=$?",
			"A\nbash: line 1: no match: zz*zz\n",
		},
		{
			// The control: an expansion's failure stops the list where it
			// stands and is the one reported.
			"an expansion failure wins",
			"echo zz*zz $((1/0)) $(echo A >&2); echo st=$?",
			"bash: line 1: 1/0: division by 0 (error token is \"0\")\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runBash(t, t.TempDir(), on+tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
