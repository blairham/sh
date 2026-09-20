// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `export` and `readonly` over a reference aimed at **one element** are taken
// here, in silence: the attribute lands on the container, because an
// attribute is a property of a name and an array has one name, and the value
// goes through the reference to the element.
//
// The opposite column to bash's, which refuses the attribute by the target's
// own text — see Semantics.ExportOrReadonlyTakesAReferenceToAnElement, where
// both are measured. What was wrong here was the **value**: it landed on
// element 0 rather than on the element the reference names, silently and at
// status 0, because the operand's name had already been replaced with the
// array's for the attribute and the value was then stored through it (#3881).
//
// Measured 2026-09-20 on ksh93u+ 2012-08-01, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME.
func TestExportAndReadonlyTakeAReferenceToAnElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[1]'; `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"export writes the element and exports the array",
			decl + `export b=Z; echo "st=$? [${a[*]}]"` + "\n" + `typeset -p a`,
			"st=0 [p Z r]\ntypeset -x -a a=(p Z r)\n",
			0,
		},
		{
			// The row bash parts from: the freeze lands, so a later write to
			// any element of the array is refused and the input ends.
			"readonly writes the element and freezes the array",
			decl + `readonly b=Y; echo "st=$? [${a[*]}]"` + "\n" +
				`a[2]=NEW; echo "after=$? [${a[*]}]"`,
			"st=0 [p Y r]\nsh: line 2: a: is read only\n",
			1,
		},
		{
			"with no value the letter still lands on the array",
			decl + `export b; echo "st=$?"` + "\n" + `typeset -p a`,
			"st=0\ntypeset -x -a a=(p q r)\n",
			0,
		},
		{
			"a table takes it the same way",
			`typeset -A m=([k]=v); typeset -n n='m[k]'; export n=Q` + "\n" +
				`echo "st=$? [${m[k]}]"; typeset -p m`,
			"st=0 [Q]\ntypeset -x -A m=([k]=Q)\n",
			0,
		},
		{
			"the control: a reference to a whole name exports that name",
			`plain=1; typeset -n n=plain; export n=Z; echo "st=$?"` + "\n" +
				`typeset -p plain`,
			"st=0\ntypeset -x plain=Z\n",
			0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != c.status {
				t.Errorf("wrote %q at %d, want %q at %d", out, status, c.want, c.status)
			}
		})
	}
}

// An append through a reference aimed at an element joins the element here
// too, which is the core's answer and not an axis: both shells that have
// references agree.
//
// Only the bare statement reaches it in this column — `typeset b+=Y` is
// `typeset: b+: invalid variable name`, which is
// Semantics.DeclarationTakesAnAppendOperand and was already right. Measured
// 2026-09-20 on ksh93u+; the value replaced the element here before #3880.
func TestAnAppendThroughAReferenceJoinsTheElement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the bare statement",
			`a=(p q r); typeset -n b='a[1]'; b+=X; echo "st=$? [${a[*]}]"`,
			"st=0 [p qX r]\n",
		},
		{
			"a table's key joins too",
			`typeset -A m=([k]=v); typeset -n n='m[k]'; n+=X; echo "st=$? [${m[k]}]"`,
			"st=0 [vX]\n",
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
