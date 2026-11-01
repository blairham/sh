// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Every word of a list is expanded before a pattern that matched nothing is
// refused, for a builtin, a function, the name, a literal and a loop alike —
// not only for a program's words (#6677). Measured 2026-10-15 on zsh 5.9.2
// under -fc and from a file, in a directory where nothing matches; every
// `want` is that shell's output. See interp.Runner.holdPathnameRefusal.
func TestNomatchRefusesAfterEveryWordIsExpanded(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a builtin's words",
			"echo $(echo S >&2) zz*zz $(echo A >&2); echo st=$?",
			"S\nA\nzsh:1: no matches found: zz*zz\n",
		},
		{
			"a function's words",
			"f() { :; }; f zz*zz $(echo A >&2); echo st=$?",
			"A\nzsh:1: no matches found: zz*zz\n",
		},
		{
			"the command's own name",
			"zz*zz $(echo A >&2); echo st=$?",
			"A\nzsh:1: no matches found: zz*zz\n",
		},
		{
			"an array literal",
			"a=(5 $(echo S >&2) zz*zz $(echo A >&2)); echo st=$?",
			"S\nA\nzsh:1: no matches found: zz*zz\n",
		},
		{
			"a for list",
			"for i in zz*zz $(echo A >&2); do echo in; done; echo st=$?",
			"A\nzsh:1: no matches found: zz*zz\n",
		},
		{
			"a qualifier that will not read",
			"echo *(Z) $(echo A >&2); echo st=$?",
			"A\nzsh:1: unknown file attribute: Z\n",
		},
		{
			"an expansion failure wins",
			"echo zz*zz $((1/0)) $(echo A >&2); echo st=$?",
			"zsh:1: division by zero\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
