// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration in a `function` body over a reference the body made itself
// stores through it, into whatever binding the reference reaches, and makes no
// local of either name. Over a reference the body did not make, the
// declaration's fresh local replaces it. That pair is the key: whose reference
// it is, not where the target lives. Measured 2026-11-02 against ksh93u+
// 2012-08-01 (`/bin/ksh`), from `-c` under `env -i PATH=/usr/bin:/bin`. This
// shell made a local of the target and left the global `x` at 0 (#7352). See
// interp.Semantics.DeclarationThroughAReferenceMakesTheTargetLocal.
func TestADeclarationThroughAFunctionsOwnReferenceStoresThroughIt(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the row", `x=0; function f { typeset -n m=x; typeset m=3; }; f; echo x=$x`, "x=3\n"},
		{
			"the reference stands",
			`x=0; function f { typeset -n m=x; typeset m=3; echo m=$m; typeset -p m; }; f; echo x=$x`,
			"m=3\ntypeset -n m=x\nx=3\n",
		},
		{
			"the pair: a global reference is replaced by a fresh local",
			`x=0; typeset -n m=x; function f { typeset m=3; echo m=$m; typeset -p m; }; f; echo x=$x`,
			"m=3\nm=3\nx=0\n",
		},
		{
			"an integer letter lands on the global target",
			`x=0; function f { typeset -n m=x; typeset -i m=3+1; echo m=$m; }; f; echo x=$x; typeset -p x`,
			"m=4\nx=4\ntypeset -i x=4\n",
		},
		{
			"a valueless declaration reads the target and keeps the reference",
			`x=0; function f { typeset -n m=x; typeset m; echo m=$m; typeset -p m; m=5; }; f; echo x=$x`,
			"m=0\ntypeset -n m=x\nx=5\n",
		},
		{
			"a valueless letter over an unset target",
			`unset x; function f { typeset -n m=x; typeset -i m; }; f; echo "x=${x-unset}"; typeset -p x`,
			"x=unset\ntypeset -i x\n",
		},
		{
			"a target local to the body stays local",
			`x=0; function f { typeset x=1; typeset -n m=x; typeset m=3; echo in=$x; }; f; echo x=$x`,
			"in=3\nx=0\n",
		},
		{
			"traced under the target",
			`x=0; function f { typeset -n m=x; typeset m=3; }; typeset -ft f; f`,
			"+ m=x\n+ typeset -n m\n+ x=3\n+ typeset m\n",
		},
		// Controls, which already agreed.
		{"a bare assignment", `x=0; function f { typeset -n m=x; m=3; }; f; echo x=$x`, "x=3\n"},
		{"a POSIX-form body", `x=0; f() { typeset -n m=x; typeset m=3; }; f; echo x=$x`, "x=3\n"},
		{"export", `x=0; function f { nameref m=x; export m=3; }; f; echo x=$x`, "x=3\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// An operand behind a declaration's operand over a reference reads the value
// where that operand landed: in the reference's target where the utility
// stores through it, and in the fresh local where the utility replaces it.
// Measured 2026-11-02 against the same ksh93u+. This shell held the value in
// the reference's own cell and read `$m` through the reference, so `v` came to
// 0 in every row (#7352).
func TestALaterOperandReadsWhereTheReferencedOperandLanded(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a fresh local over a global reference",
			`x=0; typeset -n m=x; function f { typeset m=3 v=$m; echo v=$v; }; f; echo x=$x`,
			"v=3\nx=0\n",
		},
		{
			"traced",
			`x=0; typeset -n m=x; function f { typeset m=3 v=$m; }; typeset -ft f; f`,
			"+ m=3\n+ v=3\n+ typeset m v\n",
		},
		{
			"through the body's own reference",
			`x=0; function f { typeset -n m=x; typeset m=3 v=$m; echo v=$v; }; f; echo x=$x`,
			"v=3\nx=3\n",
		},
		{"through a reference at the top level", `x=0; typeset -n m=x; typeset m=3 v=$m; echo v=$v x=$x`, "v=3 x=3\n"},
		{
			"through a reference by export",
			`x=0; typeset -n m=x; function f { export m=3 v=$m; echo v=$v; }; f; echo x=$x`,
			"v=3\nx=3\n",
		},
		// The control: no reference at all.
		{"a plain name", `m=0; function f { typeset m=3 v=$m; echo v=$v; }; f; echo m=$m`, "v=3\nm=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
