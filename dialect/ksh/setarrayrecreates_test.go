// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// This column answers `set -A` the opposite way round from zsh on two of the
// five rows, which is what makes the grid discriminating rather than a
// restatement of one shell — #4768.
//
// Measured 2026-09-27 on `/bin/ksh`, `Version AJM 93u+ 2012-08-01`, from a
// script file under `env -i PATH=/usr/bin:/bin`; `go version -m` says *not a
// Go executable* for it.
//
// The minus form re-creates the name whatever it was holding, where zsh keeps
// the attribute over an array; the plus form re-creates nothing, where zsh
// takes it off a scalar. Both are the answers those columns already hold for
// `a=(…)` and `a+=(…)`, which is why no row of this needed an axis of its own.
func TestSetArrayDropsTheAttributesAReCreationDrops(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the minus form over a scalar re-creates",
			`export a=1; set -A a x y; typeset -p a`,
			"typeset -a a=(x y)\n",
		},
		{
			"and over an array it is holding, it still does",
			`export a=(p q); set -A a x y; typeset -p a`,
			"typeset -a a=(x y)\n",
		},
		{
			"and over a declared array holding nothing",
			`typeset -a a; export a; set -A a x; typeset -p a`,
			"typeset -a a=(x)\n",
		},
		{
			"the plus form over a scalar does not",
			`export a=1; set +A a x y; typeset -p a`,
			"typeset -x -a a=(x y)\n",
		},
		{
			"nor over an array it is holding",
			`export a=(p q); set +A a x y; typeset -p a`,
			"typeset -x -a a=(x y)\n",
		},
		{
			"the integer letter goes with it, so nothing is folded",
			`typeset -i b=1; set -A b 5+5; typeset -p b`,
			"typeset -a b=(5+5)\n",
		},
		{
			"and so does one on a declared array",
			`typeset -a -i b; set -A b 5+5 6+6; typeset -p b`,
			"typeset -a b=(5+5 6+6)\n",
		},
		{
			"a store that does not happen takes nothing off",
			`export e=(p q); set +A e; typeset -p e`,
			"typeset -x -a e=(p q)\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The literal controls, which say the `set -A` rows above follow the axes
// rather than a rule of their own: the *declared empty array* is the one
// place the two routes part in this column, and it parts in both directions
// at once — the literal keeps the attribute and `set -A` takes it off.
func TestTheLiteralPartsFromSetArrayOverADeclaredEmptyArray(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a literal over a declared empty array keeps it", `typeset -a a; export a; a=(x); typeset -p a`, "typeset -x -a a=(x)\n"},
		{"a literal over an exported array drops it", `export a=(p q); a=(x y); typeset -p a`, "typeset -a a=(x y)\n"},
		{"an append over an exported scalar keeps it", `export a=1; a+=(x y); typeset -p a`, "typeset -x -a a=(1 x y)\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
