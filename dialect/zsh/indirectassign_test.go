// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The dialect has the flag, so the real lines that need it run here.
//
// The substrate's tests name the flag group; this one names the shell, and it
// is worth having because the line below is the one a real startup writes and
// because its damage was two steps away from the construct. A plugin manager
// records a file's time with
//
//	.zi-get-mtime-into() { : ${(P)2::="$(stat -f %m "$1")"} }
//	.zi-get-mtime-into "$file" 'ZI[mtime-side]'
//
// and this shell read the `(P)` on the way out only: the value went to a
// parameter *called* `2` instead of to `ZI[mtime-side]`. Nothing complained.
// Then every later `$2` in a function called with one argument read the
// leftover, the manager's "were two components given?" test answered yes, a
// plugin directory was assembled out of `user/plugin` and the leftover
// instead of `user---plugin`, and the eighteen functions autoloaded from it
// were `function definition file not found` on every startup (#1672).
//
// So the last two rows assert the *absence* of the leak, which is the half a
// test of the assignment alone would not have caught.
func TestTheIndirectionFlagAssignsThroughTheNameInThisDialect(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the plugin manager's line, at the assignment",
			`typeset -gA ZI; ` +
				`into() { : ${(P)2::="$1"}; }; ` +
				`into 1776478236 'ZI[mtime-side]'; ` +
				`printf "[%s]" "$ZI[mtime-side]"`,
			"[1776478236]",
		},
		{
			"and the parameter it was called by is not the one written",
			`typeset -gA ZI; ` +
				`into() { : ${(P)2::="$1"}; }; ` +
				`into 1776478236 'ZI[mtime-side]'; ` +
				`two() { printf "[%s][%s]" "$#" "$2"; }; ` +
				`two one`,
			"[1][]",
		},
		{
			"so a two-components test still answers no",
			`typeset -gA ZI; ` +
				`into() { : ${(P)2::="$1"}; }; ` +
				`into 1776478236 'ZI[mtime-side]'; ` +
				`split() { if [[ -n $2 ]]; then printf "[two]"; else printf "[%s]" "${1%%/*}---${1#*/}"; fi; }; ` +
				`split z-shell/z-a-meta-plugins`,
			"[z-shell---z-a-meta-plugins]",
		},
		{
			"a scalar name, on all three states of the parameter it resolves to",
			`n=t; unset t; printf "[%s]" "${(P)n::=a}"; t=; printf "[%s]" "${(P)n::=b}"; ` +
				`t=old; printf "[%s]" "${(P)n::=c}"; printf "[%s][%s]" "$t" "$n"`,
			"[a][b][c][c][t]",
		},
		{
			"and an element of this dialect's arrays, whose base is one",
			`a=(p q); i="a[2]"; printf "[%s][%s]" "${(P)i}" "${(P)i::=Z}"; printf "[%s]" "${a[@]}"`,
			"[q][Z][p][Z]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
