// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestReadArrayIntoAnIndexedElementRefills is
// interp.Semantics.ReadArrayIntoAnElementRefillsTheArray in this shell:
// `read -A 'b[i]'` takes the element out and fills the array from its first
// element on, keeping the rest, where this shell stored nothing at all. The
// subscript is read once and refused as `read 'b[i]'` refuses it, and the
// nested rows are the element holding an array of its own, which is not the
// axis. Measured 2026-10-16, 2026-10-27 and 2026-10-28 on ksh93u+ 2012-08-01,
// `-c` (#6869).
func TestReadArrayIntoAnIndexedElementRefills(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the second element", `b=(x y); read -A 'b[1]' <<< 'p q'; typeset -p b`, "typeset -a b=(p q)\n"},
		{"the first element", `b=(x y); read -A 'b[0]' <<< 'p q'; typeset -p b`, "typeset -a b=(p q)\n"},
		{"the last, one word", `b=(x y z); read -A 'b[2]' <<< p; typeset -p b`, "typeset -a b=(p y)\n"},
		{"the rest is kept", `b=(x y z w); read -A 'b[1]' <<< p; typeset -p b`, "typeset -a b=([0]=p [2]=z [3]=w)\n"},
		{"more words than the hole", `b=(x y z w); read -A 'b[3]' <<< 'p q'; typeset -p b`, "typeset -a b=(p q z)\n"},
		{"an empty line", `b=(x y z w); read -A 'b[1]' <<< ''; typeset -p b`, "typeset -a b=([0]='' [2]=z [3]=w)\n"},
		{"past the end", `b=(x y z w); read -A 'b[7]' <<< p; typeset -p b`, "typeset -a b=(p y z w)\n"},
		{"counted from the end", `b=(x y z); read -A 'b[-1]' <<< 'p q'; typeset -p b`, "typeset -a b=(p q)\n"},
		{"an empty subscript is 0", `b=(x y z); read -A 'b[]' <<< p; typeset -p b`, "typeset -a b=(p y z)\n"},
		{
			"the subscript is read once",
			`b=(x y z); i=1; read -A 'b[i++]' <<< p; typeset -p b; echo $i`,
			"typeset -a b=([0]=p [2]=z)\n2\n",
		},
		{"a name holding nothing", `read -A 'z[1]' <<< 'p q'; typeset -p z`, "typeset -a z=(p q)\n"},
		{"a name holding a string", `z=s; read -A 'z[1]' <<< 'p q'; typeset -p z`, "typeset -a z=(p q)\n"},
		{
			"an integer array",
			`typeset -i b=(1 2 3); read -A 'b[1]' <<< 7; typeset -p b`,
			"typeset -a -i b=([0]=7 [2]=3)\n",
		},
		{
			"an element holding an array",
			`typeset -a b; b[3]=(o1 o2); read -A 'b[3]' <<< p; typeset -p b`,
			"typeset -a b=([3]=(p) )\n",
		},
		{
			"a table's key still nests",
			`typeset -A m; m[k]=1; read -A 'm[k]' <<< 'p q'; typeset -p m`,
			"typeset -A m=([k]=(p q) )\n",
		},
		{
			"a frozen array is still refused",
			`b=(x y); readonly b; read -A 'b[1]' <<< x; echo after`,
			"ksh: read: b: is read only\nafter\n",
		},
		{
			"a subscript that will not evaluate",
			`b=(x y z); read -A 'b[1/0]' <<< p; echo "st=$?"; typeset -p b`,
			"ksh: read: 1/0: divide by zero\nst=1\ntypeset -a b=(x y z)\n",
		},
		{
			"the whole-array spelling is an expression here",
			`b=(x y z); read -A 'b[@]' c <<< 'p q'; echo "st=$?"; typeset -p b c`,
			"ksh: read: @: arithmetic syntax error\nst=1\ntypeset -a b=(x y z)\n",
		},
		{
			"before the first element",
			`b=(x y z); read -A 'b[-9]' <<< p; echo "st=$?"; typeset -p b`,
			"ksh: read: b: subscript out of range\nst=1\ntypeset -a b=(x y z)\n",
		},
		{"the whole array, the control", `b=(x y); read -A b <<< p; typeset -p b`, "typeset -a b=(p)\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
