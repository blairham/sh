// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The `(e)` flag in this shell, where the axes it leans on have this shell's
// answers rather than the suite defaults interp runs under.
//
// Three of them move the result and none of them belongs to the flag: an
// unquoted scalar is not split here, an unquoted array reference still yields
// one field per element, and the result of an expansion is not a pattern. A
// re-reading that decided any of the three itself would look right in interp
// and be wrong in every one of the rows below.
//
// Measured on zsh 5.9.2, 2026-09-09.
func TestTheReevalFlagUnderThisShellsAxes(t *testing.T) {
	const count = `f(){ printf "%d:" "$#"; printf "[%s]" "$@"; }; `
	for _, tc := range []struct{ name, src, want string }{
		{
			"a scalar holding a separator is one field, unquoted",
			count + `sp="a b"; v='$sp'; f ${(e)v}`, `1:[a b]`,
		},
		{
			"an array reference is one field per element, unquoted",
			count + `a=(p q r); v='$a'; f ${(e)v}`, `3:[p][q][r]`,
		},
		{
			"and one field quoted, joined with IFS's first character",
			count + `a=(p q r); v='$a'; f "${(e)v}"`, `1:[p q r]`,
		},
		{
			"a command substitution's result is split",
			count + `v='$(echo a; echo b)'; f ${(e)v}`, `2:[a][b]`,
		},
		{
			"a metacharacter in the value is not matched",
			count + `: > aa; : > ab; v='a*'; f ${(e)v}`, `1:[a*]`,
		},
		{
			"and neither is one a reference produced",
			count + `: > aa; : > ab; g='a*'; v='$g'; f ${(e)v}`, `1:[a*]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}

// The re-reading is a *single* pass, and this is the row that says so: `inner`
// holds the text `$deeper`, and the flag substitutes it without going on to
// substitute what it now holds.
//
// It is here as well as in interp because it is the property `_p9k_must_init`
// depends on — the pattern it builds is compared as text, and a reader that
// ran to a fixed point would compare a resolved prompt against a pattern of
// references and never match.
func TestTheReevalFlagDoesNotLoop(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		`inner='$deeper'; deeper=bottom; v='$inner'; print -rn -- "${(e)v}"`)
	if out != `$deeper` || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, `$deeper`)
	}
}
