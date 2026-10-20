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
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{name: "a plain declaration", src: decl + `typeset b=Z; echo "[${a[*]}]"`, want: "[p Z r]\n"},
		{name: "the global letter", src: decl + `f() { typeset -g b=G; }; f; echo "[${a[*]}]"`, want: "[p G r]\n"},
		// The two words the first version of this left out, each of which
		// went on writing element 0. The **attribute** on the same word was
		// the remainder, and it landed with #3881: bash withholds it and
		// names the target's own text at status 0, where ksh93u+ carries it
		// to the array exactly as this shell used to in both dialects. The
		// sentence is asserted here because it is *this* column's answer —
		// see Semantics.ExportOrReadonlyTakesAReferenceToAnElement and
		// TestExportAndReadonlyRefuseAReferenceToAnElement, where the rest of
		// the rows are.
		{
			name: "the readonly word",
			src:  decl + `readonly b=Z; echo "[${a[*]}]"`,
			want: "sh: line 1: readonly: `a[1]': not a valid identifier\n[p Z r]\n",
		},
		{
			name: "the export word",
			src:  decl + `export b=Z; echo "[${a[*]}]"`,
			want: "sh: line 1: export: `a[1]': not a valid identifier\n[p Z r]\n",
		},
		// A table's key through the same shape, which was worse than a wrong
		// cell: the value went to a *new* key named `0` and `k` kept `v`.
		{
			name: "a table's key under the readonly word",
			src:  `typeset -A m=([k]=v); typeset -n t='m[k]'; readonly t=T; echo "[${m[k]}]"`,
			want: "sh: line 1: readonly: `m[k]': not a valid identifier\n[T]\n",
		},
		{
			name: "the control: a reference to a whole name is unchanged",
			src:  `typeset -n n=plain; typeset n=Z; echo "[$plain]"`,
			want: "[Z]\n",
		},
		// And the same control under the word this change touched, which is
		// what says the redirect was narrowed to an element and not removed:
		// `readonly` through a reference to a plain name still writes and
		// freezes the target.
		{
			name: "the control under the readonly word",
			src:  `typeset -n n=plain; readonly n=Z; echo "[$plain]"; plain=9`,
			want: "[Z]\nsh: line 1: plain: readonly variable\n",
			st:   1,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != c.st {
				t.Errorf("wrote %q at %d, want %q at %d", out, status, c.want, c.st)
			}
		})
	}
}
