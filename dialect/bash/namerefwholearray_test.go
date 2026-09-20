// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A store through a reference aimed at `a[@]` is refused by the name, writes
// nothing, and costs a different amount at each route that reaches it.
//
// Measured 2026-09-20 on bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME. Every one of these walked the two
// characters on to the arithmetic evaluator here and answered `@: arithmetic
// syntax error: operand expected`, which **ends the script** — so one such
// line took every later line of the file with it (#2298's namerefs row).
func TestAWholeArraySubscriptThroughAReferenceIsBad(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[@]'; `
	for _, c := range []struct{ name, src, want string }{
		{
			"the bare assignment gives up the rest of the line",
			decl + `b=Z; echo "same=$?"` + "\n" + `echo "next=$? [${a[*]}]"`,
			"sh: line 1: a[@]: bad array subscript\nnext=1 [p q r]\n",
		},
		{
			"the star spelling answers alike",
			`a=(p q r); typeset -n s='a[*]'; s=Z` + "\n" + `echo "next=$? [${a[*]}]"`,
			"sh: line 1: a[*]: bad array subscript\nnext=1 [p q r]\n",
		},
		{
			"a declaration reports and the line runs on",
			decl + `typeset b=Z; echo "same=$? [${a[*]}]"`,
			"sh: line 1: a[@]: bad array subscript\nsame=1 [p q r]\n",
		},
		{
			"an arithmetic command answers for itself",
			decl + `(( b = 5 )); echo "same=$? [${a[*]}]"`,
			"sh: line 1: a[@]: bad array subscript\nsame=0 [p q r]\n",
		},
		{
			"and so does let, which is that command under a builtin's name",
			decl + `let 'b = 6'; echo "same=$? [${a[*]}]"`,
			"sh: line 1: a[@]: bad array subscript\nsame=0 [p q r]\n",
		},
		{
			"a table refuses too, where the bare m[@]=Z stores the key",
			`typeset -A m=([k]=v); typeset -n n='m[@]'; n=Z` + "\n" +
				`echo "next=$? [${m[@]-}][${m[k]}]"`,
			"sh: line 1: m[@]: bad array subscript\nnext=1 [v][v]\n",
		},
		{
			"the control: the same table takes the key written bare",
			`typeset -A m=([k]=v); m[@]=Z; echo "same=$? [${m[@]}]"`,
			"same=0 [Z v]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := answersRun(t, c.src)
			if out != c.want {
				t.Errorf("wrote %q, want %q", out, c.want)
			}
		})
	}
}

// A declaration's **value** goes to the element a reference names, where its
// attributes go to the array. Measured 2026-09-20 on bash 5.3.20 with
// `a=(p q r); typeset -n b='a[1]'` in front of each; every one of these wrote
// element 0 here, silently and at 0. See Runner.referenceValueTarget.
func TestADeclarationsValueFollowsAReferenceToTheElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[1]'; `
	for _, c := range []struct{ name, src, want string }{
		{"a plain declaration", decl + `typeset b=Z; echo "[${a[*]}]"`, "[p Z r]\n"},
		{"the global letter", decl + `f() { typeset -g b=G; }; f; echo "[${a[*]}]"`, "[p G r]\n"},
		{
			"the control: a reference to a whole name is unchanged",
			`typeset -n n=plain; typeset n=Z; echo "[$plain]"`, "[Z]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != 0 {
				t.Errorf("wrote %q at %d, want %q at 0", out, status, c.want)
			}
		})
	}
}
