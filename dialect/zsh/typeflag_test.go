// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${(t)name}` is what a name *is*, in this shell's own words.
//
// The mechanics belong to interp/typeflag_test.go, which supplies a
// vocabulary of its own; these are the *words*, which are this dialect's and
// are the one thing that file cannot assert. They are deliberately the same
// function `$parameters` renders with, so the two spellings of one question
// cannot answer it differently — the last row is what holds that.
//
// Measured against zsh 5.9.2, 2026-09-10; the corpus row is
// `param/expansion-flags-parameter-type`.
func TestTheTypeFlagWordsWhatANameIs(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `v=abc; print -r -- ${(t)v}`, "scalar\n"},
		{"an array", `w=(a b); print -r -- ${(t)w}`, "array\n"},
		{"an association", `typeset -A m=(k1 v1); print -r -- ${(t)m}`, "association\n"},
		{"an integer", `typeset -i n=1; print -r -- ${(t)n}`, "integer\n"},
		// `-E` is the other float spelling and is refused by name here, so
		// only `-F` is asserted; the day it lands, `${(t)b}` is `float` too.
		{"a float", `typeset -F a=1; print -r -- ${(t)a}`, "float\n"},
		{"exported", `typeset -x e=1; print -r -- ${(t)e}`, "scalar-export\n"},
		{"an exported array", `typeset -xa a=(1); print -r -- ${(t)a}`, "array-export\n"},
		{"readonly before export", `typeset -xr v=1; print -r -- ${(t)v}`, "scalar-readonly-export\n"},
		{"unique after export", `typeset -xaU a=(1); print -r -- ${(t)a}`, "array-export-unique\n"},
		{"local", `f() { local l=1; print -r -- ${(t)l}; }; f`, "scalar-local\n"},
		{"local before readonly", `f() { typeset -ir l=1; print -r -- ${(t)l}; }; f`, "integer-local-readonly\n"},
		{"a tied pair", `typeset -T TV tv; print -r -- ${(t)TV} ${(t)tv}`, "scalar-tied array-tied\n"},
		// The two hiding letters are two attributes with two words, which
		// is the whole of #2042: a parameter given only `-H` is `hideval`
		// and not `hide`, so the four rows are the two letters alone, the
		// pair, and where the pair sits among the other words.
		{"the value-hiding letter", `typeset -H hv=1; print -r -- ${(t)hv}`, "scalar-hideval\n"},
		{"the scope-hiding letter", `typeset -h hs=1; print -r -- ${(t)hs}`, "scalar-hide\n"},
		{"hide before hideval", `typeset -hH b=1; print -r -- ${(t)b}`, "scalar-hide-hideval\n"},
		{
			"and both after unique",
			`typeset -rHhU -a c=(1 2); print -r -- ${(t)c}`,
			"array-readonly-unique-hide-hideval\n",
		},
		// The three width letters, which said nothing here until #4504 — a
		// fact the runner already held and nothing carried across the seam.
		// `-Z` and `-R` are two spellings of one *side*, which is why the
		// word names the fill rather than the letter.
		{"the left-justifying width letter", `typeset -L v=ab; print -r -- ${(t)v}`, "scalar-left\n"},
		{"the right-justifying one", `typeset -R w=ab; print -r -- ${(t)w}`, "scalar-right_blanks\n"},
		{"and the zero-filling one, which is the same side", `typeset -Z z=ab; print -r -- ${(t)z}`, "scalar-right_zeros\n"},
		{"a width with no value to learn from still carries the letter", `typeset -L v; print -r -- ${(t)v}`, "scalar-left\n"},
		{"local before a width", `f() { typeset -L l=ab; print -r -- ${(t)l}; }; f`, "scalar-local-left\n"},
		{"a width before unique", `f() { typeset -UL l=ab; print -r -- ${(t)l}; }; f`, "scalar-local-left-unique\n"},
		{"a width before upper", `typeset -uL v=ab; print -r -- ${(t)v}`, "scalar-left-upper\n"},
		{"a width before readonly", `typeset -rL v=ab; print -r -- ${(t)v}`, "scalar-left-readonly\n"},
		{"a width before export", `typeset -xL v=ab; print -r -- ${(t)v}`, "scalar-left-export\n"},
		{"a width before hideval", `typeset -HL v=ab; print -r -- ${(t)v}`, "scalar-left-hideval\n"},
		{"and the numeric attribute keeps its own word", `typeset -iL n=5; print -r -- ${(t)n}`, "integer-left\n"},
		{"the container wins over the numeric attribute", `typeset -ia ia; ia=(1 2); print -r -- ${(t)ia}`, "array\n"},
		{"an unset name is empty, and unset", `unset u; print -r -- "[${(t)u}][${(t)u-D}]"`, "[][D]\n"},
		{
			"and it is the same word the table gives",
			`typeset -xr v=1; w=(a b); print -r -- "[${(t)v}][${parameters[v]}][${(t)w}][${parameters[w]}]"`,
			"[scalar-readonly-export][scalar-readonly-export][array][array]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
