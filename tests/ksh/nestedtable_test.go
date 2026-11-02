// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A literal whose subscripts are keys, assigned to one element, makes that
// element a keyed table: `a[k]=([u]=1 [v]=2)` stores both pairs under `k`,
// traced as the two element writes into it. This read `u` and `v` as
// arithmetic, both 0, and kept `([k]=(2) )` (#6778). Measured 2026-11-02
// against ksh93u+ 2012-08-01 (`/bin/ksh`) from `-c` under `env -i
// PATH=/usr/bin:/bin`, byte for byte.
//
// The minimal pair is the first four rows: the keyed literal nests a table,
// the plain list and a list that opens with a bare word still nest an indexed
// array, and the chained writes the trace reads like make an indexed array of
// their own — so it is the literal that makes the table, not the chain.
func TestAKeyedLiteralAssignedToAnElementNestsATable(t *testing.T) {
	t.Parallel()
	const table = `typeset -A a; a[k]=([u]=1 [v]=2); `
	for _, c := range []struct{ name, src, want string }{
		{
			"the row",
			`typeset -A a; set -x; a[k]=([u]=1 [v]=2); typeset -p a`,
			"+ a[k][u]=1\n+ a[k][v]=2\n+ typeset -p a\ntypeset -A a=([k]=([u]=1 [v]=2) )\n",
		},
		{"the pair: a plain list", `typeset -A a; a[k]=(1 2); typeset -p a`, "typeset -A a=([k]=(1 2) )\n"},
		{"the pair: a bare word first", `typeset -A a; a[k]=(1 [u]=2); typeset -p a`, "typeset -A a=([k]=(1 '[u]=2') )\n"},
		{"the pair: the chain alone", `typeset -A a; a[k][u]=1; a[k][v]=2; typeset -p a`, "typeset -A a=([k]=(2) )\n"},
		{
			"over an indexed array",
			`typeset -a a; set -x; a[1]=([u]=1); typeset -p a`,
			"+ a[1][u]=1\n+ typeset -p a\ntypeset -a a=([1]=([u]=1) )\n",
		},
		{
			"read back",
			table + `echo "${a[k][u]}|${#a[k][@]}|${!a[k][@]}|${a[k][@]}|${a[k][0]}"`,
			"1|2|u v|1 2|\n",
		},
		{
			"appended to",
			table + `set -x; a[k]+=([w]=3); set +x; typeset -p a`,
			"+ a[k][w]=3\ntypeset -A a=([k]=([u]=1 [v]=2 [w]=3) )\n",
		},
		{
			"a chained write into it takes a key",
			table + `a[k][w]=3; typeset -p a; echo "[${a[k][0]}]"`,
			"typeset -A a=([k]=([u]=1 [v]=2 [w]=3) )\n[]\n",
		},
		{
			"numbers are keys too",
			`typeset -A a; a[k]=([0]=x [2]=y); a[k][1+1]=z; typeset -p a; echo "${!a[k][@]}"`,
			"typeset -A a=([k]=([0]=x [1+1]=z [2]=y) )\n0 1+1 2\n",
		},
		{
			"traced in the order written, keys quoted",
			`typeset -A a; x=5; set -x; a[k]=([v]=2 [$x]=1 ["p q"]='r s')`,
			"+ a[k][v]=2\n+ a[k][5]=1\n+ a[k]['p q']='r s'\n",
		},
		{
			"inside a whole literal",
			`typeset -A a=([k]=([u]=1 [v]=2)); typeset -p a; echo "${a[k][v]}"`,
			"typeset -A a=([k]=([u]=1 [v]=2) )\n2\n",
		},
		{
			"a subshell's write stays there",
			table + `(a[k][w]=3; typeset -p a); typeset -p a`,
			"typeset -A a=([k]=([u]=1 [v]=2 [w]=3) )\ntypeset -A a=([k]=([u]=1 [v]=2) )\n",
		},
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
