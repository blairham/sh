// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// A declaration's bad name ends its declaring and not its assigning in this
// shell: the refused `x+=5` operand lands as the statement it spells, and the
// plain valued operands behind any bad name land without the line's letters
// or freeze. Measured 2026-10-27 on ksh93u+ 2012-08-01 under `env -i
// PATH=/usr/bin:/bin`, `-c` (#6510). See
// interp.Semantics.BadNameEndsTheDeclaringNotTheAssigning.
func TestARefusedAppendOperandIsAssignedFirst(t *testing.T) {
	for _, tc := range []struct{ name, src, has, suffix string }{
		{
			"readonly lands and does not freeze",
			`x=0; command readonly x+=5; echo "st=$? x=$x"; x=9; echo "x=$x"`,
			"readonly: x+: invalid variable name", "st=1 x=05\nx=9\n",
		},
		{
			"the line's letter does not reach it, nor the operand behind",
			`x=0; command typeset -i x+=5 z=1+1; typeset -p x z`, "typeset: x+: invalid variable name", "x=05\nz=1+1\n",
		},
		{
			"a standing attribute joins as the statement would",
			`typeset -i x=0; command readonly x+=5; typeset -p x`, "", "typeset -i x=5\n",
		},
		{
			"export lands and marks",
			`x=0; command export x+=5; typeset -p x`, "export: x+: is not an identifier", "typeset -x x=05\n",
		},
		{
			"a keyword function's readonly reaches the caller's name",
			`x=0; function f { readonly x+=5; }; f; echo "x=$x"`, "f[1]: readonly: x+: invalid variable name", "x=05\n",
		},
		{
			"static scope: the name f sees",
			`function g { typeset x=0; f; echo "g=$x"; }; function f { readonly x+=5; }; x=7; g; echo "top=$x"`,
			"", "g=0\ntop=75\n",
		},
		{
			"readonly leaves an empty unfrozen local behind the refused operand",
			`x=0; function f { command readonly x+=5; x=3; typeset -p x; }; f; echo "x=$x"`, "", "x=3\nx=05\n",
		},
		{
			"typeset lands in its own local",
			`function f { command typeset x+=5; typeset -p x; }; x=0; f; echo "out x=$x"`, "", "x=5\nout x=0\n",
		},
		{
			"the operand behind the refusal is not frozen",
			`x=0; command readonly x+=5 y; y=1; echo "y=$y"`, "", "y=1\n",
		},
		{
			"the operands in front were declared",
			`x=0; command readonly y=2 x+=5; y=1; echo "y=$y"`, "y: is read only", "",
		},
		{
			"the array literal form is unchanged",
			`x=(a b); function f { readonly x+=(c); }; f; typeset -p x`, "x+: invalid variable name", "typeset -a x=(a b c)\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKshValues(t, tc.src)
			if (tc.has != "" && !strings.Contains(out, tc.has)) || !strings.HasSuffix(out, tc.suffix) {
				t.Errorf("%s = %q, want it to hold %q and end %q", tc.src, out, tc.has, tc.suffix)
			}
		})
	}
}

// The same past a plain bad name, per word: `readonly`'s values land unfrozen
// on the name the function sees, `typeset`'s in its own local with no letter,
// and `export`'s exported — while a valueless or subscripted operand behind
// the refusal does nothing at all.
func TestTheValuesBehindABadNameLandWithoutTheDeclaring(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"readonly, not frozen", `command readonly 1y z=2; z=1; echo "z=$z"`, "z=1\n"},
		{"readonly, in front is frozen", `command readonly z=2 1y; z=1; echo NOT`, "z: is read only\n"},
		{"typeset -i, no letter", `command typeset -i 1y z=1+1; typeset -p z`, "z=1+1\n"},
		{"typeset -x, not exported", `command typeset -x 1y z=2; typeset -p z`, "z=2\n"},
		{"export, exported", `command export 1y z=2; typeset -p z`, "typeset -x z=2\n"},
		{"export, valueless not exported", `z=1; command export 1y z; typeset -p z`, "z=1\n"},
		{"subscripted stores nothing", `command readonly 1y "a[1]=v"; echo "c=${#a[@]}"`, "c=0\n"},
		{"typeset's local", `function f { command typeset 1y z=2; typeset -p z; }; f; echo "out z=[$z]"`, "z=2\nout z=[]\n"},
		{"readonly's caller value, no local", `function f { command readonly 1y z=2; typeset -p z; }; f; echo "out z=$z"`, "z=2\nout z=2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runKshValues(t, tc.src)
			if !strings.HasSuffix(out, tc.want) {
				t.Errorf("%s = %q, want it to end %q", tc.src, out, tc.want)
			}
		})
	}
}

// The store is ahead of the sentence, so a frozen name's refusal is the
// line's whole complaint and the bad name is never judged — for the append
// operand and for a plain bad name alike. The operands in front have landed.
func TestAFrozenStoreIsTheWholeComplaint(t *testing.T) {
	for _, tc := range []struct{ src, lacks, want string }{
		{`readonly x=0; function f { readonly x+=5; }; f; echo after`, "x+", "f: line 1: x: is read only\nafter\n"},
		{`readonly x=0; command readonly 1y x=5; echo "st=$?"`, "1y", "x: is read only\n"},
		{`readonly x=0; function f { readonly z=1 x+=5; }; f; echo "z=$z"`, "x+", "f: line 1: x: is read only\nz=1\n"},
	} {
		out, _ := runKshValues(t, tc.src)
		if strings.Contains(out, tc.lacks) || !strings.Contains(out, tc.want) {
			t.Errorf("%s = %q, want %q in it and no %q", tc.src, out, tc.want, tc.lacks)
		}
	}
}

func runKshValues(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}
