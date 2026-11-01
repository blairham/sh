// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration utility's operand sees the value its assignment prefix has just
// expanded to: the prefix is worked through first, and each value is under its
// name by the time the operand expands. The prefix is still taken back once
// the utility has run. Measured 2026-10-29 against zsh 5.9.2
// (`/opt/homebrew/bin/zsh`) under `env -i PATH=/usr/bin:/bin`, from `-c` and
// from a script file alike. This shell expanded the prefix first and kept the
// values aside, so the operand read the name as the command found it (#6999).
//
// The controls are rows the shell already answered: a child sees the value
// only where the name was exported already, `builtin typeset` reads its words
// as an ordinary command's, `eval` and `typeset -i` read the name once the
// prefix is in place, and an ordinary command's argument never sees it. See
// interp/prefixoperandorder.go.
func TestADeclarationsOperandSeesItsPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `v=old; v=new typeset y=$v; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"export", `v=old; v=new export y=$v; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"readonly", `v=old; v=new readonly y=$v; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"declare", `v=old; v=new declare y=$v; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"an unset name", `unset v; v=new typeset y=$v; echo "y=$y"`, "y=new\n"},
		{"in a function", `f() { v=new typeset y=$v; echo "in f y=$y"; }; v=old; f`, "in f y=new\n"},
		{"local", `f() { v=new local y=$v; echo "in f y=$y"; }; v=old; f`, "in f y=new\n"},
		{
			"in a function-form function",
			`function f { v=new typeset y=$v; echo "in f y=$y v=$v"; }; v=old; f`,
			"in f y=new v=old\n",
		},
		{"every operand", `v=old; v=new typeset y=$v z=$v; echo "y=$y z=$z v=$v"`, "y=new z=new v=old\n"},
		{"every entry", `v=old; a=1 v=new b=2 typeset y=$v$a$b; echo "y=$y"`, "y=new12\n"},
		{"a substitution in the operand", `v=old; v=new typeset y=$(echo "$v"); echo "y=$y"`, "y=new\n"},
		{
			"the prefix's substitution first",
			`v=old; v=$(echo pre >&2; echo new) typeset y=$(echo op >&2; echo $v); echo "y=$y"`,
			"pre\nop\ny=new\n",
		},
		// Operands are not stored in turn here, so one over the prefix's own
		// name leaves the prefix's value in sight on either side of it.
		{"an operand over the prefix's own name", `v=old; v=new typeset v=x y=$v; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"an operand behind one over the same name", `v=old; v=new typeset y=$v v=x; echo "y=$y v=$v"`, "y=new v=old\n"},
		{"an array literal operand", `v=old; v=new typeset -a a=($v); echo "a=${a[@]}"`, "a=new\n"},
		{"every pass of a loop", `for i in 1 2 3; do v=$i typeset y=$v; echo "y=$y"; done`, "y=1\ny=2\ny=3\n"},
		{
			"a child, where the name was exported",
			`export v=old; v=new typeset y=$(/usr/bin/printenv v); echo "y=$y"`,
			"y=new\n",
		},
		// Controls.
		{"a child, where it was not", `v=old; v=new export y=$(/usr/bin/printenv v); echo "y=$y"`, "y=\n"},
		{"builtin typeset", `v=old; v=new builtin typeset y=$v; echo "y=$y v=$v"`, "y=old v=old\n"},
		{"eval", `v=old; v=new eval 'typeset y=$v'; echo "y=$y"`, "y=new\n"},
		{"the letters", `v=old; v=2 typeset -i y=v+1; echo "y=$y"`, "y=3\n"},
		{"an ordinary argument", `v=old; v=new echo "$v"`, "old\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// The command's trace line writes the operand as it expanded behind the
// prefix. Measured the same day and the same way.
func TestADeclarationsTracedOperandSeesItsPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"scalars",
			`set -x; v=old; v=new typeset y=$v z=$v`,
			"+zsh:1> v=old \n+zsh:1> v=new +zsh:1> typeset y=new z=new\n",
		},
		{
			"an array literal",
			`set -x; v=old; v=new typeset -a a=($v)`,
			"+zsh:1> v=old \n+zsh:1> v=new +zsh:1> typeset -a a=( new )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}
