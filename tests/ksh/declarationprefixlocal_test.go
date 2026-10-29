// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A prefix in front of `typeset` inside a `function`-form body is that body's
// own local. A function it calls does not see it, and the caller gets its own
// value back at the return. Measured 2026-10-29 against ksh93u+ 2012-08-01
// (`/bin/ksh`) under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script
// file alike. This shell stored the prefix in the cell the name already
// reached, which was the global (#7002).
//
// The controls are rows the shell already answered: in front of `readonly`,
// `export` or `:` the prefix stays an ordinary assignment, and so does a
// POSIX-form body's. A local the body already has, or a reference it already
// holds, is written as it stands. See Semantics.PrefixToALocalDeclarationIsALocal.
func TestAPrefixToTypesetInAFunctionIsItsLocal(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a function the body calls",
			`function g { echo "g v=$v"; }; function f { v=new typeset y=1; echo "in v=$v"; g; }; v=old; f; echo "out v=$v"`,
			"in v=new\ng v=old\nout v=old\n",
		},
		{
			"an unset name",
			`function f { v=new typeset y=1; echo "in v=$v"; }; f; echo "out v=${v-unset}"`,
			"in v=new\nout v=unset\n",
		},
		{"a valueless operand", `function f { v=new typeset y; }; v=old; f; echo "out v=$v"`, "out v=old\n"},
		{
			"an operand over the prefix's name",
			`function f { v=new typeset v=x; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=x\nout v=old\n",
		},
		{
			"an operand that reads it",
			`function f { v=new typeset y=$v; echo "in y=$y v=$v"; }; v=old; f; echo "out v=$v y=${y-unset}"`,
			"in y=new v=new\nout v=old y=unset\n",
		},
		{
			"a function called from the operand",
			`function g { echo "g:$v"; }; function f { v=new typeset y=$(g); echo "y=$y"; }; v=old; f`,
			"y=g:old\n",
		},
		{
			"a function called from a later entry",
			`function g { echo "$v"; }; function f { v=new w=$(g) typeset y; echo "w=$w v=$v"; }; v=old; f; echo "out v=$v"`,
			"w=old v=new\nout v=old\n",
		},
		{
			"through command",
			`function f { v=new command typeset y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		{
			"a listing",
			`function f { v=new typeset -p y; echo "in v=$v"; }; v=old; y=7; f; echo "out v=$v"`,
			"y=7\nin v=new\nout v=old\n",
		},
		{
			"integer",
			`function f { v=new integer y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		{
			"eval",
			`function f { eval 'v=new typeset y=1'; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		{
			"a nested call",
			`function h { v=new typeset y=1; echo "h v=$v"; }; function f { h; echo "f v=$v"; }; v=old; f; echo "out v=$v"`,
			"h v=new\nf v=old\nout v=old\n",
		},
		{
			"a POSIX-form body inside one",
			`g() { v=new typeset y=1; echo "g v=$v"; }; function f { g; echo "f v=$v"; }; v=old; f; echo "out v=$v"`,
			"g v=new\nf v=new\nout v=old\n",
		},
		{
			"every entry",
			`function f { a=1 b=$a typeset y=1; echo "in a=$a b=$b"; }; a=0; f; echo "out a=$a b=${b-unset}"`,
			"in a=1 b=1\nout a=0 b=unset\n",
		},
		{
			"an append joins a fresh cell",
			`function f { v+=x typeset y=$v; echo "in v=$v y=$y"; }; v=old; f; echo "out v=$v"`,
			"in v=x y=x\nout v=old\n",
		},
		{
			"the value before the cell",
			`function f { v=${v}x typeset y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=oldx\nout v=old\n",
		},
		{
			"no letters",
			`typeset -i v=1; function f { v=2+3 typeset y=1; echo "in v=$v"; typeset -p v; }; f; echo "out v=$v"`,
			"in v=2+3\nv=2+3\nout v=1\n",
		},
		{
			"not exported",
			`export v=old; function f { v=new typeset y=1; /usr/bin/printenv v; typeset -p v; }; f; echo "out v=$v"; /usr/bin/printenv v`,
			"v=new\nout v=old\nold\n",
		},
		{
			"a child of the operand",
			`export v=old; function f { v=new typeset y=$(/usr/bin/printenv v); echo "y=$y"; }; f`,
			"y=\n",
		},
		{
			"a child of a later entry",
			`export v=old; function f { v=new w=$(/usr/bin/printenv v) typeset y; echo "w=$w"; }; f`,
			"w=\n",
		},
		{
			"an array",
			`function f { v=(a b) typeset y=1; echo "in ${v[1]}"; }; v=old; f; echo "out v=$v"`,
			"in b\nout v=old\n",
		},
		{
			"an element",
			`a=(p q r); function f { a[1]=B typeset y=1; echo "in ${#a[@]} ${!a[@]} ${a[*]}"; }; f; echo "out ${a[*]}"`,
			"in 1 1 B\nout p q r\n",
		},
		{
			"a reference the shell holds",
			`typeset -n r=v; function f { r=new typeset y=1; echo "in r=$r v=$v"; }; v=old; f; echo "out v=$v"`,
			"in r=new v=old\nout v=old\n",
		},
		{
			"a subshell",
			`v=old; function f { v=new typeset y=1; echo "in v=$v"; }; (f; echo "sub v=$v"); echo "out v=$v"`,
			"in v=new\nsub v=old\nout v=old\n",
		},
		{
			"a call with a prefix of its own",
			`function f { v=new typeset y=1; echo "in v=$v"; }; v=old; v=x f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		// Controls.
		{
			"readonly",
			`function f { v=new readonly y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=new\n",
		},
		{"export", `function f { v=new export y=1; }; v=old; f; echo "out v=$v"`, "out v=new\n"},
		{":", `function f { v=new :; echo "in v=$v"; }; v=old; f; echo "out v=$v"`, "in v=new\nout v=new\n"},
		{"a POSIX-form body", `f() { v=new typeset y=1; }; v=old; f; echo "out v=$v"`, "out v=new\n"},
		{"the top level", `v=old; v=new typeset y=1; echo "top v=$v"`, "top v=new\n"},
		{
			"the body's own local",
			`function f { typeset v=mine; v=new typeset y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=old\n",
		},
		{
			"the body's own reference",
			`function f { typeset -n r=v; r=new typeset y=1; echo "in v=$v"; }; v=old; f; echo "out v=$v"`,
			"in v=new\nout v=new\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != 0 || out != c.want {
				t.Errorf("got %q (status %d), want %q", out, st, c.want)
			}
		})
	}
}

// When the operand walk stops inside such a body, the prefix it had stored
// goes with the call: it was the body's local. Measured the same day and the
// same way. The EXIT trap row is the one #6999 moved away from ksh93u+, by
// keeping a stopped walk's prefix in the global cell. The operands behind the
// prefix are not pinned here: they stay as globals, which is #7011.
func TestAStoppedDeclarationInAFunctionTakesItsPrefixAway(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"an EXIT trap",
			`trap 'echo "trap v=$v"' EXIT; function f { v=new typeset y=${q?boom}; }; v=old; f`,
			"ksh: f: line 1: q: boom\ntrap v=old\n", 1,
		},
		{
			"the line after the call",
			`function f { v=new typeset y=${q?boom}; echo in; }; v=old; f; echo "out v=$v"`,
			"ksh: f: line 1: q: boom\nout v=old\n", 0,
		},
		{
			"a frozen local",
			`function f { typeset -r r=0; v=new typeset x=$v r=2; echo in; }; v=old; f; echo "out v=$v"`,
			"ksh: f: line 1: r: is read only\nout v=old\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), c.src)
			if st != c.status || out != c.want {
				t.Errorf("got %q (status %d), want %q at %d", out, st, c.want, c.status)
			}
		})
	}
}
