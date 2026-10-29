// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A prefix in front of `local` persists here, because `local` is a special
// builtin, and it is an ordinary assignment and not a local of the function: a
// function the body calls sees it and the caller keeps it. These rows are the
// other side of the one where ksh93 makes a prefix in front of `typeset` the
// body's own local. Measured 2026-10-29 against dash 0.5.12 under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file alike. See
// Semantics.PrefixToALocalDeclarationIsALocal (#7002).
func TestAPrefixToLocalIsAnOrdinaryAssignment(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a function the body calls",
			`g() { echo "g v=$v"; }; f() { v=new local y=1; echo "in v=$v"; g; }; v=old; f; echo "out v=$v"`,
			"in v=new\ng v=new\nout v=new\n",
		},
		{
			"an unset name",
			`f() { v=new local y; echo "in v=$v"; }; unset v; f; echo "out v=${v-unset}"`,
			"in v=new\nout v=new\n",
		},
		{
			"the body's own local",
			`f() { local v=mine; v=new local y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		{
			"export",
			`f() { v=new export y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=new\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
