// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A subscript is not a quoting context in this dialect, and a backslash in a
// pattern reaches only a metacharacter. One rule and its pattern half.
//
// The substrate's tests name the axes; these rows name the shell, because it
// is the only one that answers this way and the answers are what make a
// script's key or search operand mean what it wrote. Measured 2026-09-07 on
// zsh 5.9.2 with a scratch HOME and ZDOTDIR.
func TestASubscriptIsNotAQuotingContextInThisDialect(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			"a quoted key keeps its quotes",
			`typeset -A m; m["k"]=W; kk='"k"'; printf '[%s]' "${m[$kk]}" "${m[k]}"`,
			`[W][]`,
		},
		{
			"a substitution in a key is performed and the quotes are not removed",
			`typeset -A q; q[k]=K; v=k; printf '[%s]' "${q[$v]}" "${q["$v"]}"`,
			`[K][]`,
		},
		{
			"and the same on the writing side",
			`typeset -A m; m["k"]=W; printf '%d' "${#m[@]}"; kk='"k"'; printf '[%s]' "${m[$kk]}"`,
			`1[W]`,
		},
		{
			"a search operand keeps a backslash before an ordinary character",
			`a=('bet\a' beta); printf '[%s]' "${a[(r)bet\a]}"`,
			`[bet\a]`,
		},
		{
			"so it does not find the element without one",
			`b=(beta gamma); printf '[%s]' "${b[(r)bet\a]}"`,
			`[]`,
		},
		{
			"and before a metacharacter the backslash still escapes",
			`c=(bex 'be*'); printf '[%s]' "${c[(r)be\*]}" "${c[(r)be*]}"`,
			`[be*][bex]`,
		},
		// The guards. A bare `@` is still the whole array, a key still keeps
		// its blanks, and neither moves with the axis.
		{
			"a bare at is the whole array and a quoted one is a key",
			`typeset -A n; n[a]=1; n[b]=2; printf '%d' "${#n[@]}"; printf '[%s]' "${n["@"]}"`,
			`2[]`,
		},
		{
			"a key keeps its blanks",
			`typeset -A p; p[s]=T; printf '[%s]' "${p[ s ]}" "${p[s]}"`,
			`[][T]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
