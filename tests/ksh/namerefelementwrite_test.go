// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// An element written through a reference that is itself aimed at an element
// is the chain the two subscripts spell: `e[k]=1` with `e` aimed at `a[x]` is
// `a[x][k]=1`, so the element becomes an indexed array and `k` is arithmetic.
// Measured 2026-11-02 against ksh93u+ 2012-08-01 (`/bin/ksh`), from `-c`
// under `env -i PATH=/usr/bin:/bin`, byte for byte. This shell refused the
// write as `a[x]: invalid variable name` and ended the shell (#7113).
//
// The minimal pair is the first two rows: an unset name and a number as the
// script's subscript land under 0 and under 1 of the nested array, which is
// what says the subscript is arithmetic in the element and not a key of the
// table. The indexed row says the nesting is not the table's alone, and the
// plain `e=1` is the control that already agreed: with no subscript of the
// script's own, the write lands in the element.
func TestAnElementWriteThroughAnElementReferenceNestsTheElement(t *testing.T) {
	t.Parallel()
	const table = `typeset -A a; typeset -n e='a[x]'; `
	for _, c := range []struct{ name, src, want string }{
		{
			"the row: a name as the subscript",
			table + `e[k]=1; echo "st=$?"; typeset -p a; echo "${a[x][0]}"`,
			"st=0\ntypeset -A a=([x]=(1) )\n1\n",
		},
		{
			"the pair: a number as the subscript",
			table + `e[1]=1; echo "st=$?"; typeset -p a; echo "${a[x][1]}"`,
			"st=0\ntypeset -A a=([x]=([1]=1) )\n1\n",
		},
		{"traced as the chain", `set -x; ` + table + `e[k]=1`, "+ typeset -A a\n+ e='a[x]'\n+ typeset -n e\n+ a[x][0]=1\n"},
		{
			"an indexed array",
			`typeset -a a; typeset -n e='a[1]'; e[2]=z; echo "st=$?"; typeset -p a; echo "${a[1][2]}"`,
			"st=0\ntypeset -a a=([1]=([2]=z) )\nz\n",
		},
		{
			"a string in the element is promoted",
			`a=(p q r); typeset -n e='a[1]'; e[2]=z; typeset -p a`,
			"typeset -a a=(p ([0]=q [2]=z) r)\n",
		},
		{"appends", table + `e[k]=1; e[k]+=2; e[1]+=z; typeset -p a`, "typeset -A a=([x]=(12 z) )\n"},
		{"through a second reference", table + `typeset -n f=e; f[2]=1; typeset -p a`, "typeset -A a=([x]=([2]=1) )\n"},
		{
			"a key that needs quoting, traced",
			`typeset -A a; typeset -n e='a[x y]'; set -x; e[1]=v`,
			"+ a['x y'][1]=v\n",
		},
		// Controls, which already agreed.
		{"the plain write lands in the element", table + `e=1; echo "st=$?"; typeset -p a`, "st=0\ntypeset -A a=([x]=1)\n"},
		{"a reference aimed at a name", `typeset -n e=v; e[1]=z; typeset -p v`, "typeset -a v=([1]=z)\n"},
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
