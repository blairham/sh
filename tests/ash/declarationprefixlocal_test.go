// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import "testing"

// A prefix in front of `local` persists here, because `local` is a special
// builtin, and it is an ordinary assignment and not a local of the function,
// as in dash. Measured 2026-10-29 against BusyBox v1.37.0 in the digest-pinned
// alpine image under `env -i PATH=/usr/bin:/bin`. See
// Semantics.PrefixToALocalDeclarationIsALocal (#7002).
func TestAPrefixToLocalIsAnOrdinaryAssignment(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the caller keeps it",
			`f() { v=new local y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=new\n",
		},
		{
			"a function the body calls",
			`g() { echo "g v=$v"; }; f() { v=new local y=1; echo "in v=$v"; g; }; v=old; f; echo "out v=$v"`,
			"in v=new\ng v=new\nout v=new\n",
		},
		{
			"the body's own local",
			`f() { local v=mine; v=new local y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := run(t, c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
