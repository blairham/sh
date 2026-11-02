// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Under `failglob` a declaration's words are all expanded before a pattern
// that matched nothing is refused, behind an assignment operand as in front of
// one: bash has no order of its own there, which the zsh reading of #7125
// must not reach. Measured 2026-11-02 on bash 5.3.20 under `env -i
// PATH=/usr/bin:/bin`, in a directory where nothing matches; every `want` is
// that shell's output with both streams on one pipe.
func TestFailglobRefusesADeclarationAfterEveryWordIsExpanded(t *testing.T) {
	t.Parallel()
	const on = "shopt -s failglob; "
	for _, tc := range []struct{ name, src, want string }{
		{
			"behind an assignment operand",
			"declare y=1 zz*zz $(echo S >&2); echo st=$?",
			"S\nbash: line 1: no match: zz*zz\n",
		},
		{
			"ahead of an assignment operand",
			"declare zz*zz y=1 $(echo S >&2); echo st=$?",
			"S\nbash: line 1: no match: zz*zz\n",
		},
		{
			"local, behind an assignment operand",
			"f() { local y=1 zz*zz $(echo S >&2); }; f; echo st=$?",
			"S\nbash: line 1: no match: zz*zz\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), on+tc.src)
			if out != tc.want {
				t.Errorf("out %q\nwant %q", out, tc.want)
			}
		})
	}
}
