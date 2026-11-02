// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A subscript read through a reference that is itself aimed at an element is
// the chain the two subscripts spell, as the write is: with `e` aimed at
// `a[x]`, `${e[0]}` is `${a[x][0]}` and `${#e[@]}` counts the nested element.
// Measured 2026-11-02 against ksh93u+ 2012-08-01 (`/bin/ksh`), from `-c` under
// `env -i PATH=/usr/bin:/bin`, byte for byte. This shell read nothing through
// the reference (#7366): the first two rows were `r0= n=0 d=1` and `r= n=0`.
//
// The plain `$e` reads the element itself, nested or not, as `${a[x]}` does;
// and an element holding a string is itself at the base and holds no elements,
// which is what `${a[1][0]}` and `${#a[1][@]}` already answer.
func TestAnElementReadThroughAnElementReferenceReadsTheNestedElement(t *testing.T) {
	t.Parallel()
	const table = `typeset -A a; typeset -n e='a[x]'; `
	for _, c := range []struct{ name, src, want string }{
		{
			"the row: a table's element",
			table + `e[k]=1; echo "r0=${e[0]} n=${#e[@]} d=${a[x][0]}"`,
			"r0=1 n=1 d=1\n",
		},
		{
			"the row: an indexed array's element",
			`typeset -a a; typeset -n e='a[1]'; e[2]=z; echo "r=${e[2]} n=${#e[@]}"`,
			"r=z n=1\n",
		},
		{
			"values, the join, subscripts and count",
			table + `e[k]=1; e[3]=q; echo "all=<${e[@]}> star=<${e[*]}> keys=<${!e[@]}> n=${#e[@]}"`,
			"all=<1 q> star=<1 q> keys=<0 3> n=2\n",
		},
		{
			"one field per element",
			table + `e[k]=1; e[2]=b; for v in "${e[@]}"; do echo "[$v]"; done`,
			"[1]\n[b]\n",
		},
		{
			"the operators read the chain",
			table + `e[k]=1; echo "<${e[0]:-d}> <${e[5]:-d}> <${e[5]-u}> <${e[0]-u}> <${#e[0]}> <${e[0]/1/Z}>"`,
			"<1> <d> <u> <1> <1> <Z>\n",
		},
		{"through a second reference", table + `e[k]=1; typeset -n f=e; echo "<${f[0]}> <${#f[@]}>"`, "<1> <1>\n"},
		{
			"a key that needs quoting",
			`typeset -A a; typeset -n e='a[x y]'; e[1]=v; echo "<${e[1]}> <${#e[@]}> <${!e[@]}>"`,
			"<v> <1> <1>\n",
		},
		{"the plain read of a nested element", table + `e[k]=1; echo "<$e> <${a[x]}>"`, "<1> <1>\n"},
		{
			"the plain read of a compound element",
			`typeset -A a; a[x]=(p=1); typeset -n e='a[x]'; echo "<$e> <${a[x]}>"`,
			"<(\n\tp=1\n)> <(\n\tp=1\n)>\n",
		},
		{
			"an element holding a string",
			`a=(p q r); typeset -n e='a[1]'; echo "<$e> <${e[0]}> <${e[1]}> n=${#e[@]} <${e[@]}> <${!e[@]}>"`,
			"<q> <q> <> n=0 <> <>\n",
		},
		{
			"a table's element holding a string",
			`typeset -A a; a[x]=s; typeset -n e='a[x]'; echo "<$e> <${e[0]}> <${e[1]}>"`,
			"<s> <s> <>\n",
		},
		{
			"the subscripts of the chain written out",
			`typeset -A a; a[x][0]=1; a[x][3]=q; echo "${!a[x][@]}"`,
			"0 3\n",
		},
		// Controls, which already agreed.
		{
			"a reference aimed at a name",
			`typeset -n e=v; v=(p q); echo "<${e[1]}> <${#e[@]}> <${!e[@]}>"`,
			"<q> <2> <0 1>\n",
		},
		{"an element never written", table + `echo "<$e> <${e[0]}> <${#e[@]}>"`, "<> <> <0>\n"},
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
