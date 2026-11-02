// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A declaration in a function over a reference the function made itself
// follows the reference here too, and makes the target a local of the call,
// where ksh93 writes the global target (#7352). And the operands are expanded
// before any is stored, so a later one reads what the command started with.
// Measured 2026-11-02 against bash 5.3.20 from `-c` under `env -i
// PATH=/usr/bin:/bin`. See
// interp.Semantics.DeclarationThroughAReferenceMakesTheTargetLocal.
func TestADeclarationThroughAFunctionsOwnReferenceMakesTheTargetLocal(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"declare",
			`x=0; f() { declare -n m=x; declare m=3; declare -p m x; }; f; echo x=$x`,
			"declare -n m=\"x\"\ndeclare -- x=\"3\"\nx=0\n",
		},
		{"local", `x=0; f() { local -n m=x; local m=3; }; f; echo x=$x`, "x=0\n"},
		{"an integer letter", `x=0; f() { declare -n m=x; declare -i m=3+1; }; f; echo x=$x`, "x=0\n"},
		{"valueless", `x=0; f() { declare -n m=x; declare m; echo m=$m; m=5; }; f; echo x=$x`, "m=\nx=0\n"},
		{
			"a later operand, through the function's own reference",
			`x=0; f() { declare -n m=x; declare m=3 v=$m; echo v=$v; }; f; echo x=$x`,
			"v=0\nx=0\n",
		},
		{
			"a later operand, over a global reference",
			`x=0; declare -n m=x; f() { declare m=3 v=$m; echo v=$v; declare -p m; }; f; echo x=$x`,
			"v=0\ndeclare -- m=\"3\"\nx=0\n",
		},
		{"a later operand, at the top level", `x=0; declare -n m=x; declare m=3 v=$m; echo v=$v x=$x`, "v=0 x=3\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runBash(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
