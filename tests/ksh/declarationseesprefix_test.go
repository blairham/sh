// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration utility's operand sees the value its assignment prefix has just
// expanded to: the prefix is worked through first, and each value is under its
// name by the time the operand expands. Measured 2026-10-29 against ksh93u+
// 2012-08-01 (`/bin/ksh`) under `env -i PATH=/usr/bin:/bin`, from `-c` and
// from a script file alike. This shell expanded the prefix first and kept the
// values aside, so the operand read the name as the command found it (#6999).
//
// The controls are rows the shell already answered: an operand over the
// prefix's own name stands over it, `eval` and `typeset -i` read the name once
// the prefix is in place, and an ordinary command's argument never sees it.
// See interp/prefixoperandorder.go.
func TestADeclarationsOperandSeesItsPrefix(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"typeset", `v=old; v=new typeset y=$v; echo "y=$y v=$v"`, "y=new v=new\n"},
		{"export", `v=old; v=new export y=$v; echo "y=$y v=$v"`, "y=new v=new\n"},
		{"readonly", `v=old; v=new readonly y=$v; echo "y=$y v=$v"`, "y=new v=new\n"},
		{"an unset name", `unset v; v=new typeset y=$v; echo "y=$y"`, "y=new\n"},
		{"in a POSIX-form function", `f() { v=new typeset y=$v; echo "in f y=$y"; }; v=old; f`, "in f y=new\n"},
		{
			"in a function-form function",
			`function f { v=new typeset y=$v; echo "in f y=$y v=$v"; }; v=old; f`,
			"in f y=new v=new\n",
		},
		{"every operand", `v=old; v=new typeset y=$v z=$v; echo "y=$y z=$z v=$v"`, "y=new z=new v=new\n"},
		{"every entry", `v=old; a=1 v=new b=2 typeset y=$v$a$b; echo "y=$y"`, "y=new12\n"},
		{"a substitution in the operand", `v=old; v=new typeset y=$(echo "$v"); echo "y=$y"`, "y=new\n"},
		{
			"the prefix's substitution first",
			`v=old; v=$(echo pre >&2; echo new) typeset y=$(echo op >&2; echo $v); echo "y=$y"`,
			"pre\nop\ny=new\n",
		},
		{"an operand behind one over the same name", `v=old; v=new typeset y=$v v=x; echo "y=$y v=$v"`, "y=new v=x\n"},
		{"an array literal operand", `v=old; v=new typeset -a a=($v); echo "a=${a[@]}"`, "a=new\n"},
		{"a literal and a scalar", `v=old; v=new typeset -a a=($v) y=$v; echo "a=${a[@]} y=$y"`, "a=new y=new\n"},
		{"through command", `v=old; v=new command typeset y=$v; echo "y=$y v=$v"`, "y=new v=new\n"},
		{"every pass of a loop", `for i in 1 2 3; do v=$i typeset y=$v; echo "y=$y"; done`, "y=1\ny=2\ny=3\n"},
		// Controls.
		{"an operand over the prefix's own name", `v=old; v=new typeset v=x y=$v; echo "y=$y v=$v"`, "y=x v=x\n"},
		{"eval", `v=old; v=new eval 'typeset y=$v'; echo "y=$y"`, "y=new\n"},
		{"the letters", `v=old; v=2 typeset -i y=v+1; echo "y=$y"`, "y=3\n"},
		{"an ordinary argument", `v=old; v=new echo "$v"`, "old\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// The prefix's line comes before the literal operand's, and the literal is
// written with what the prefix came to. Measured the same day and the same
// way. A scalar operand's line is not pinned here: it is written in front of
// the prefix's, which is a separate defect (#7001).
func TestADeclarationsTracedLiteralSeesItsPrefix(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `set -x; v=old; v=new typeset -a a=($v)`)
	if want := "+ v=old\n+ v=new\n+ a=( new )\n+ typeset -a a\n"; st != 0 || out != want {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// When the operand walk stops, the prefix stays with the operands it reached:
// the walk had stored the prefix ahead of the first of them. Measured the same
// day and the same way from `-c`, with an EXIT trap as the reader. The middle row is the
// control, which this shell answered already: an operand over the prefix's
// name stands over it.
func TestAStoppedDeclarationKeepsItsPrefix(t *testing.T) {
	const trap = `trap 'echo "trap v=$v x=$x"' EXIT; `
	for _, c := range []struct{ name, src, want string }{
		{
			"a frozen operand",
			trap + `readonly r=0; v=old; v=new typeset x=$v r=2`,
			"ksh: r: is read only\ntrap v=new x=new\n",
		},
		{
			"a frozen operand behind one over the prefix's name",
			trap + `readonly r=0; v=old; v=new typeset v=1 x=$v r=2`,
			"ksh: r: is read only\ntrap v=1 x=1\n",
		},
		{
			"a value that will not expand",
			trap + `v=old; v=new typeset x=$v y=${q?boom}`,
			"ksh: q: boom\ntrap v=new x=new\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 1 || out != c.want {
				t.Errorf("got %q (status %d), want %q at 1", out, st, c.want)
			}
		})
	}
}
