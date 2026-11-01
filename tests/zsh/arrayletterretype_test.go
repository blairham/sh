// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnArrayLetterInFrontOfALiteralRetypesAFrozenScalar pins that `typeset
// -a v=(x)` over a frozen scalar is taken exactly as `typeset v=(x)` is: the
// name becomes a frozen array holding the literal. The letter lands before
// the literal is stored, so the store used to see an array and refuse its
// own line as a write to a frozen array. The controls are the two that stay
// refused: a name that was already an array before the line, and a bare
// literal afterwards.
//
// Every row measured 2026-10-15 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`
// under `env -i PATH=/usr/bin:/bin`, from `-c` and from a script file),
// output compared byte for byte (#6514).
func TestAnArrayLetterInFrontOfALiteralRetypesAFrozenScalar(t *testing.T) {
	t.Parallel()
	const ro = "typeset -r v=1; "
	const refused = "zsh:1: read-only variable: v\n"
	for _, tc := range []struct{ name, src, want string }{
		{"typeset -a", ro + "typeset -a v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly\n"},
		{"typeset -A", ro + "typeset -A v=(k x); echo after $? ${(kv)v} ${(t)v}", "after 0 k x association-readonly\n"},
		{"over a name holding nothing", "readonly v; typeset -a v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly\n"},
		{"readonly -a", ro + "readonly -a v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly\n"},
		{"export -a", ro + "export -a v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly-export\n"},
		{"local -a", ro + "local -a v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly\n"},
		{"local -a in a function", "f() { typeset -r v=1; local -a v=(x); echo in $? $v ${(t)v}; }; f", "in 0 x array-local-readonly\n"},
		{"an empty literal", ro + "typeset -a v=(); echo after $? ${(t)v}", "after 0 array-readonly\n"},
		{"two operands", "typeset -r v=1 w=2; typeset -a v=(x) w=(y); echo after $? $v $w ${(t)w}", "after 0 x y array-readonly\n"},
		{"the same line twice", ro + "typeset -a v=(x); typeset -a v=(y); echo after", refused},

		// The controls.
		{"no letter", ro + "typeset v=(x); echo after $? $v ${(t)v}", "after 0 x array-readonly\n"},
		{"already an array", "typeset -ra v=(a); typeset -a v=(x); echo after", refused},
		{"already a table", "typeset -A v; typeset -r v; typeset -A v=(k x); echo after", refused},
		{"a bare literal afterwards", ro + "typeset -a v=(x); v=(y); echo after", refused},
		{"local -a over a frozen array", "f() { typeset -ra v=(a); local -a v=(x); echo in; }; f", "f: read-only variable: v\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := runZshOnPath(t, t.TempDir(), tc.src)
			if got != tc.want {
				t.Errorf("%s\ngot  %q\nwant %q", tc.src, got, tc.want)
			}
		})
	}
}
