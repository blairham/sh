// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// interp.Semantics.ReadArrayIntoAnElementRefillsTheArray is no here: `read
// -A 'b[i]'` lands the words where `b[i]=(words)` lands them — a splice over
// the element, padded out to it past the end, a span replaced whole, put in
// front of everything before the first element — and is refused over a
// string, over a table's key and at 0 in the splice's own words, which end
// the script as the freeze's refusal does from inside `read`. Measured
// 2026-10-27 and 2026-10-28 on zsh 5.9.2, `-c` (#6869).
func TestReadArrayIntoAnElementSplices(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"the first element", `b=(x y); read -A 'b[1]' <<< 'p q'; typeset -p b`, "typeset -a b=( p q y )\n"},
		{"a middle element", `b=(x y z); read -A 'b[2]' <<< 'p q'; typeset -p b`, "typeset -a b=( x p q z )\n"},
		{"the last element", `b=(x y z); read -A 'b[3]' <<< p; typeset -p b`, "typeset -a b=( x y p )\n"},
		{"past the end", `b=(x y z); read -A 'b[5]' <<< p; typeset -p b`, "typeset -a b=( x y z '' p )\n"},
		{"an empty line", `b=(x y z); read -A 'b[2]' <<< ''; typeset -p b`, "typeset -a b=( x '' z )\n"},
		{"counted from the end", `b=(x y z); read -A 'b[-1]' <<< 'p q'; typeset -p b`, "typeset -a b=( x y p q )\n"},
		{"before the first element", `b=(x y z); read -A 'b[-5]' <<< p; typeset -p b`, "typeset -a b=( p x y z )\n"},
		{"a span", `b=(x y z); read -A 'b[2,3]' <<< 'p q'; typeset -p b`, "typeset -a b=( x p q )\n"},
		{"the whole-array spelling", `b=(x y z); read -A 'b[*]' <<< 'p q'; typeset -p b`, "typeset -a b=( p q )\n"},
		{
			"the subscript is read once",
			`b=(x y z); i=1; read -A 'b[i++]' <<< p; typeset -p b; echo $i`,
			"typeset -a b=( p y z )\n2\n",
		},
		{"a name holding nothing", `read -A 'z[2]' <<< 'p q'; typeset -p z`, "typeset -a z=( '' p q )\n"},
		{
			"a string",
			`z=s; read -A 'z[2]' <<< 'p q'; echo after`,
			"zsh:1: z: attempt to assign array value to non-array\n",
		},
		{
			"a table's key",
			`typeset -A m; m[k]=1; read -A 'm[k]' <<< 'p q'; echo after`,
			"zsh:1: m: attempt to set slice of associative array\n",
		},
		{
			"element zero",
			`b=(x y z); read -A 'b[0]' <<< p; echo after`,
			"zsh:1: b: assignment to invalid subscript range\n",
		},
		{"a subscript that will not evaluate", `b=(x y z); read -A 'b[1/0]' <<< p; echo after`, "zsh:1: division by zero\n"},
		{"an empty subscript", `b=(x y z); read -A 'b[]' <<< p; echo after`, "zsh:1: not an identifier: b[]\n"},
		{"the whole array, the control", `b=(x y); read -A b <<< p; typeset -p b`, "typeset -a b=( p )\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
