// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A declaration utility's operand is expanded before its assignment prefix
// here, so it reads the name as the command found it. The controls for #6999,
// where the two columns that work through the prefix first hand the operand
// the prefix's value. Measured 2026-10-29 against dash 0.5.12 under `env -i
// PATH=/usr/bin:/bin`, from `-c` and from a script file alike. See
// interp/prefixoperandorder.go.
func TestADeclarationsOperandDoesNotSeeItsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"export", `v=old; v=new export y=$v; echo "y=$y v=$v"`, "y=old v=new\n"},
		{"readonly", `v=old; v=new readonly y=$v; echo "y=$y v=$v"`, "y=old v=new\n"},
		{"an unset name", `unset v; v=new export y=$v; echo "y=$y"`, "y=\n"},
		{"local", `f() { v=new local y=$v; echo "in f y=$y v=$v"; }; v=old; f; echo "out v=$v"`, "in f y=old v=new\nout v=new\n"},
		{
			"the operand's substitution first",
			`v=old; v=$(echo pre >&2; echo new) export y=$(echo op >&2; echo $v); echo "y=$y"`,
			"op\npre\ny=old\n",
		},
		{"traced", `set -x; v=old; v=new export y=$v z=$v`, "+ v=old\n+ v=new export y=old z=old\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runDash(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
