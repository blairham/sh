// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFrozenArrayKeepsItsElementsHere: the refusal an indexed element write
// earns leaves the element as it was, where `(( a[1]=5 ))` said `read-only
// variable: a` and wrote 5 regardless; and of the routes into a frozen
// *table*, the arithmetic ones are the Yes side of
// interp.Semantics.ArithStoreIntoAFrozenTableIsTaken while `getopts` and
// `${a[k]::=x}` are refused (#6826). Measured 2026-10-16 on zsh 5.9.2, `-f`.
func TestAFrozenArrayKeepsItsElementsHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"(( )) indexed", `a=(1 2); readonly a; (( a[1]=5 )); echo "st=$?"; typeset -p a`,
			"zsh:1: read-only variable: a\nst=2\ntypeset -ar a=( 1 2 )\n",
		},
		{
			"let indexed", `a=(1 2); readonly a; let 'a[1]=5'; echo "st=$?"; typeset -p a`,
			"zsh:1: read-only variable: a\nst=1\ntypeset -ar a=( 1 2 )\n",
		},
		{
			"(( )) table is taken", `typeset -A a; readonly a; (( a[k]=5 )); echo "st=$?"; typeset -p a`,
			"st=0\ntypeset -Ar a=( [k]=5 )\n",
		},
		{
			"let table is taken", `typeset -A a; readonly a; let 'a[k]=5'; echo "st=$?"; typeset -p a`,
			"st=0\ntypeset -Ar a=( [k]=5 )\n",
		},
		{
			"getopts table", `typeset -A a; readonly a; getopts x 'a[k]' -x; echo after`,
			"zsh:1: read-only variable: a\n",
		},
		{
			"an assigning default", `typeset -A a; readonly a; : ${a[k]::=x}; echo after`,
			"zsh:1: read-only variable: a\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
