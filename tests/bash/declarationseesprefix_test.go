// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A declaration utility's operand is expanded before its assignment prefix
// here, so it reads the name as the command found it. The controls for #6999,
// where the two columns that work through the prefix first hand the operand
// the prefix's value. Measured 2026-10-29 against GNU bash 5.3.20 and bash
// 3.2.57 under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script file
// alike. See interp/prefixoperandorder.go.
func TestADeclarationsOperandDoesNotSeeItsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `v=old; v=new typeset y=$v; echo "y=$y v=$v"`, "y=old v=old\n"},
		{"export", `v=old; v=new export y=$v; echo "y=$y v=$v"`, "y=old v=old\n"},
		{"readonly", `v=old; v=new readonly y=$v; echo "y=$y v=$v"`, "y=old v=old\n"},
		{"declare", `v=old; v=new declare y=$v; echo "y=$y v=$v"`, "y=old v=old\n"},
		{"an unset name", `unset v; v=new typeset y=$v; echo "y=$y"`, "y=\n"},
		{"local", `f() { v=new local y=$v; echo "in f y=$y"; }; v=old; f`, "in f y=old\n"},
		{"every operand", `v=old; v=new typeset y=$v z=$v; echo "y=$y z=$z v=$v"`, "y=old z=old v=old\n"},
		{
			"the operand's substitution first",
			`v=old; v=$(echo pre >&2; echo new) typeset y=$(echo op >&2; echo $v); echo "y=$y"`,
			"op\npre\ny=old\n",
		},
		{"an array literal operand", `v=old; v=new typeset -a a=($v); echo "a=${a[@]}"`, "a=old\n"},
		{"every pass of a loop", `for i in 1 2 3; do v=$i typeset y=$v; echo "y=$y"; done`, "y=\ny=\ny=\n"},
		{"eval", `v=old; v=new eval 'typeset y=$v'; echo "y=$y"`, "y=new\n"},
		{"traced", `set -x; v=old; v=new typeset y=$v z=$v`, "+ v=old\n+ v=new\n+ typeset y=old z=old\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
