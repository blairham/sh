// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestAFrozenArrayKeepsItsElementsThroughEveryRouteHere: `let` and `getopts`
// wrote an indexed element through a refusal and a table's element with no
// refusal at all, and `${a[k]:=x}` did the second too (#6826). Measured
// 2026-10-16 on ksh93u+ 2012-08-01, `-c`.
func TestAFrozenArrayKeepsItsElementsThroughEveryRouteHere(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"let indexed", `a=(1 2); readonly a; let 'a[1]=5'; echo "st=$?"; typeset -p a`,
			"ksh: let: a: is read only\nst=1\ntypeset -r -a a=(1 2)\n",
		},
		{
			"getopts indexed", `a=(1 2); readonly a; getopts x 'a[1]' -x; echo "st=$?"; typeset -p a`,
			"ksh: ksh: a: is read only\nst=2\ntypeset -r -a a=(1 2)\n",
		},
		{"(( )) table", `typeset -A a; readonly a; (( a[k]=5 )); echo after`, "ksh: a: is read only\n"},
		{"a default table", `typeset -A a; readonly a; : ${a[k]:=x}; echo after`, "ksh: a: is read only\n"},
		{
			"let table", `typeset -A a=([j]=1); readonly a; let 'a[j]=5'; echo "st=$?"; typeset -p a`,
			"ksh: let: a: is read only\nst=1\ntypeset -r -A a=([j]=1)\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAnExpressionsReadOfAMissingKeyLeavesItDeclared: the key an expression's
// read leaves behind is the valueless declaration's, listed `[q]=` rather than
// `[q]=”`, and every read of the empty key warns under `set -x`, not only the
// one that creates it (#6779). Measured 2026-10-16 on ksh93u+ 2012-08-01.
func TestAnExpressionsReadOfAMissingKeyLeavesItDeclared(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the listing", "typeset -A a\n(( a[\"q\"] ))\n(( a[r] + 1 ))\nx=$(( a[s] ))\n: ${a[t]}\nlet a[u]\ntypeset -p a\n",
			"typeset -A a=([q]= [r]= [s]= [u]=)\n",
		},
		{"and an assigned empty", "typeset -A a\n(( a[q] ))\na[r]=\ntypeset -p a\n", "typeset -A a=([q]= [r]='')\n"},
		{
			"each read warns", "typeset -A a\nset -x\nx=$(( a[\"\"] ))\nx=$(( a[\"\"] ))\n",
			"ksh: line 3: warning: adding empty subscript\n+ x=0\nksh: line 4: warning: adding empty subscript\n+ x=0\n",
		},
		{
			"an existing key warns", "typeset -A a\na[\"\"]=1\nset -x\nx=$(( a[\"\"] ))\n",
			"ksh: line 4: warning: adding empty subscript\n+ x=1\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
