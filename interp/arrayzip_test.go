// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// zipping turns on the grammar these tests need, by the construct's name
// rather than by a shell's.
func zipping(d *syntax.Dialect) {
	d.ParamArrayZip = true
	d.ParamExpansionFlags = true
	d.ArrayLiteral = true
}

// A bare name standing for its elements is a *semantics* axis and not part of
// the grammar, so it is answered here rather than assumed. The zip's operand
// is a bare name by construction — there is nowhere to write a subscript —
// which is why these tests need the axis where the element-selection ones
// could reach the same list through `${(@)a…}` instead.
func bareNamesAreLists(r *Runner) {
	r.Semantics.ArrayNameWithoutSubscriptIsTheList = Yes
	r.Semantics.ArrayScalarIsTheWholeArray = Yes
}

// `${a:^b}` interleaves the value with the array `b` names, and `${a:^^b}`
// keeps going by cycling the shorter of the two.
//
// The operand is a **name** and not a word: `${a:^b}` reads the array stored
// under `b`, exactly as the set operators do, and a name nothing is stored
// under contributes no second list at all — which leaves the left operand
// untouched, the opposite of what a set difference does with the same
// absence.
func TestAZipInterleavesTwoLists(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"equal lengths alternate", `a=(1 2 3); b=(x y z); printf "[%s]" ${a:^b}`, "[1][x][2][y][3][z]"},
		{"the shorter one ends it", `a=(1 2); b=(x y z w); printf "[%s]" ${a:^b}`, "[1][x][2][y]"},
		{"either side may be the shorter", `a=(1 2 3 4); b=(x y); printf "[%s]" ${a:^b}`, "[1][x][2][y]"},
		{"cycling carries the shorter round", `a=(1 2); b=(x y z w); printf "[%s]" ${a:^^b}`, "[1][x][2][y][1][z][2][w]"},
		{"and it cycles whichever is shorter", `a=(1 2 3 4); b=(x); printf "[%s]" ${a:^^b}`, "[1][x][2][x][3][x][4][x]"},
		{"a scalar is the one-element list it is", `s=one; b=(x y); printf "[%s]" ${s:^b}`, "[one][x]"},
		{"an unset operand leaves the left alone", `a=(1 2); printf "[%s]" ${a:^nosuch}`, "[1][2]"},
		{"and cycling leaves it alone too", `a=(1 2); printf "[%s]" ${a:^^nosuch}`, "[1][2]"},
		{"an empty operand has nothing to cycle", `a=(1 2); b=(); printf "[%s]" ${a:^^b}`, "[1][2]"},
		{"a flag group applies to what the zip made", `a=(1 2); b=(x y); printf "[%s]" ${(j:-:)a:^b}`, "[1-x-2-y]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runGrammar(t, tc.src, zipping, bareNamesAreLists); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// Counted rather than printed, because `printf` runs its format once even
// with nothing to fill it — `[]` comes out both from an empty list and from a
// list of one empty string, and those are different answers. That is what
// made the quoted empty case below read as already correct while this
// interpreter was producing no field where the shell produces one.
//
// Quoting picks the **operand's** shape and not the result's: the left side
// is joined to one word before the zip sees it, and the answer is still a
// list. What quoting does guarantee is that an empty answer is one empty
// field rather than none, which is the same rule the element filter keeps.
func TestAQuotedZipJoinsTheLeftAndStillYieldsFields(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the left is one word and the answer is two fields",
			`a=(1 2 3); b=(x y z); set -- "${a:^b}"; printf "n=%d" "$#"; printf "<%s>" "$@"`,
			"n=2<1 2 3><x>",
		},
		{
			"an empty answer is no field unquoted",
			`a=(1 2 3); b=(); set -- ${a:^b}; printf "n=%d" "$#"`,
			"n=0",
		},
		{
			"and one empty field quoted",
			`a=(1 2 3); b=(); set -- "${a:^b}"; printf "n=%d" "$#"; printf "<%s>" "$@"`,
			"n=1<>",
		},
		{
			// Set-and-empty is not never-set: an empty array quoted is one
			// empty word, which the zip pairs with the other list's first
			// element; a name that was never set is no word to pair at all.
			"a set-but-empty left is one empty word",
			`a=(); b=(x y); set -- "${a:^b}"; printf "n=%d" "$#"; printf "<%s>" "$@"`,
			"n=2<><x>",
		},
		{
			"where a name never set is none",
			`b=(x y); set -- "${a:^b}"; printf "n=%d" "$#"; printf "<%s>" "$@"`,
			"n=1<>",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runGrammar(t, tc.src, zipping, bareNamesAreLists); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
