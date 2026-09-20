// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A declaration's **value** goes to the element a reference names, where its
// attributes go to the array holding it. An attribute is a property of a name
// and an array has one name; the value on the same word still belongs to the
// one cell.
//
// Pinned here as well as in bash's own suite because ksh93 answers the whole
// row and bash does not: measured 2026-09-20 against AT&T ksh93u+ 2012-08-01
// (`/bin/ksh` here), script files under `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, this column carries the attribute to the array *and* writes
// the element, silently and at 0, where bash writes the element but withholds
// the attribute and complains about the name. So the listings below are the
// tighter evidence, and they are what says the value landed in the cell the
// script named rather than beside it.
//
// `readonly` and `export` were left out when the redirect was written for
// `typeset` and each of them went on writing element **0**: `readonly b=Z`
// left `p q r` as `Z q r`, and the table row was worse than a wrong cell —
// the value made a *new* key named `0` and `k` kept `v`. See
// Runner.referenceValueTarget.
func TestADeclarationsValueFollowsAReferenceToTheElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[1]'; `
	for _, c := range []struct{ name, src, want string }{
		{
			"the readonly word freezes the array and writes the element",
			decl + `readonly b=Z; typeset -p a`,
			"typeset -r -a a=(p Z r)\n",
		},
		{
			"the export word likewise",
			decl + `export b=Z; typeset -p a`,
			"typeset -x -a a=(p Z r)\n",
		},
		{
			"a table's key, where the value used to make a key named 0",
			`typeset -A m=([k]=v); typeset -n t='m[k]'; readonly t=T; typeset -p m`,
			"typeset -r -A m=([k]=T)\n",
		},
		// The control for the redirect itself: a reference aimed at a whole
		// name is not an element, so the value goes to the target and the
		// freeze goes with it. It must not move.
		{
			"the control: a reference to a whole name",
			`u=1; typeset -n s=u; readonly s=Z; typeset -p u`,
			"typeset -r u=Z\n",
		},
		// And the word the redirect was first written for, which is what
		// says this change narrowed nothing that already worked.
		{
			"the control: the word that already carried the value",
			decl + `typeset b=Z; typeset -p a`,
			"typeset -a a=(p Z r)\n",
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
