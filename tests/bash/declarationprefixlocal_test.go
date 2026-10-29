// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// No prefix is kept in front of a declaration here, in either function form
// and under `set -o posix` too, so whether a kept one would be the function's
// local is never asked. The `export` row is the control: a special builtin's
// prefix is kept under the mode. Measured 2026-10-29 against GNU bash 5.3.20
// and bash 3.2.57 under `env -i PATH=/usr/bin:/bin`, from `-c` and from a
// script file alike. See Semantics.PrefixToALocalDeclarationIsALocal (#7002).
func TestNoPrefixIsKeptInFrontOfADeclarationInAFunction(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"local", `f() { v=new local y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`, "in v=old\nout v=old\n"},
		{"declare", `f() { v=new declare y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`, "in v=old\nout v=old\n"},
		{
			"typeset in a function-form body",
			`function f { v=new typeset y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=old\nout v=old\n",
		},
		{
			"local under the mode",
			`set -o posix; f() { v=new local y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=old\nout v=old\n",
		},
		{
			"export under the mode",
			`set -o posix; f() { v=new export y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=new\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
