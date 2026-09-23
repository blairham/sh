// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// A bare assignment's subscript closes at the bracket that balances the one it
// opened, so a `]` with no `=` behind it ends the *name* rather than sending the
// scan looking for a later one.
//
// Measured 2026-09-23 from a script file under `env -i PATH=/usr/bin:/bin
// LC_ALL=C bash f.sh`, standard input on the null device, bash 5.3.20 — and
// unanimous with zsh 5.9.2 and ksh93u+, which is what makes it core:
//
//	m[a]b]=z    `m[a]b]=z: command not found`, status 127
//	a[1][2]=v   the same shape, where the chain is not this grammar's
//	a[x[y]]=v   the key is the four characters `x[y]`
//	a[x[y]c]=v  the key is `x[y]c`
//
// The search this replaced took the first `]` that had an `=` behind it, so
// `m[a]b]=z` quietly stored a key spelled `a]b` at status 0 and `a[1][2]=v`
// reached the arithmetic reader with `1][2` in it (#4241).
func TestABareAssignmentsSubscriptBalancesItsBrackets(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a closer with no operator behind it is not an assignment",
			`declare -A m; m[a]b]=z; printf '[%s][%s]' "$?" "${#m[@]}"`,
			"[127][0]",
		},
		{
			"a chain is not this grammar's",
			`declare -a a=(1 2); a[1][2]=v; printf '[%s][%s]' "$?" "${a[1]}"`,
			"[127][2]",
		},
		{
			"a nested pair inside one subscript is part of the key",
			`declare -A k; k[x[y]]=v; printf '[%s]' "${k['x[y]']}"`,
			"[v]",
		},
		{
			"and so is text behind it",
			`declare -A c; c[x[y]c]=v; printf '[%s]' "${c['x[y]c']}"`,
			"[v]",
		},
		{
			"the ordinary spelling is untouched",
			`declare -A p; p[q]=1; printf '[%s]' "${p[q]}"`,
			"[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if !strings.HasSuffix(out, tc.want) || st != 0 {
				t.Errorf("= %q status %d, want it to end %q", out, st, tc.want)
			}
		})
	}
}
