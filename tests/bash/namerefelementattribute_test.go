// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `export` and `readonly` over a reference aimed at **one element** refuse
// the attribute by the target's own text, at status 0, and let the value
// through to the element.
//
// Measured 2026-09-20 on bash 5.3.20, script files under `env -i
// PATH=/usr/bin:/bin` with a scratch HOME, `a=(p q r); typeset -n b='a[1]'`
// in front of each. This shell was silent, wrote element **0**, and put the
// letter on the whole array — which for `readonly` left an array bash never
// freezes frozen, so the next ordinary write to any element was refused
// (#3881). See Semantics.ExportOrReadonlyTakesAReferenceToAnElement.
func TestExportAndReadonlyRefuseAReferenceToAnElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[1]'; `
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			"export writes the element and refuses the letter",
			decl + `export b=Z; echo "st=$? [${a[*]}]"` + "\n" + `declare -p a`,
			"sh: line 1: export: `a[1]': not a valid identifier\n" +
				"st=0 [p Z r]\n" +
				`declare -a a=([0]="p" [1]="Z" [2]="r")` + "\n",
			0,
		},
		{
			"readonly writes the element and freezes nothing",
			decl + `readonly b=Y; echo "st=$? [${a[*]}]"` + "\n" +
				`a[2]=NEW; echo "after=$? [${a[*]}]"`,
			"sh: line 1: readonly: `a[1]': not a valid identifier\n" +
				"st=0 [p Y r]\nafter=0 [p Y NEW]\n",
			0,
		},
		{
			"with no value at all, nothing is declared either",
			decl + `export b; echo "st=$?"` + "\n" + `declare -p a`,
			"sh: line 1: export: `a[1]': not a valid identifier\n" +
				"st=0\n" + `declare -a a=([0]="p" [1]="q" [2]="r")` + "\n",
			0,
		},
		{
			"a table names its key back the same way",
			`typeset -A m=([k]=v); typeset -n n='m[k]'; export n=Q` + "\n" +
				`echo "st=$? [${m[k]}]"`,
			"sh: line 1: export: `m[k]': not a valid identifier\nst=0 [Q]\n",
			0,
		},
		{
			// The `n` letter takes the **sentence** away and not the
			// refusal: an `a` that was exported before the line is still
			// exported after it, so nothing was applied either way. Reading
			// the letter as "then do the ordinary thing" removed the
			// attribute instead, which is the row that tells the two
			// readings apart.
			"the n letter is silent and still applies nothing",
			`a=(p q r); declare -x a; typeset -n b='a[1]'; export -n b; echo "st=$?"` + "\n" +
				`declare -p a`,
			"st=0\n" + `declare -ax a=([0]="p" [1]="q" [2]="r")` + "\n",
			0,
		},
		{
			"readonly's n letter is silent too, and the value still lands",
			decl + `readonly -n b=Z; echo "st=$? [${a[*]}]"` + "\n" +
				`a[0]=W; echo "after=$? [${a[*]}]"`,
			"st=0 [p Z r]\nafter=0 [W Z r]\n",
			0,
		},
		{
			// The control that says this is the two builtins' name check
			// and not the letter: `-x` written on a declaration lands on the
			// array, silently, exactly as it does in the column that takes
			// the operand.
			"the control: declare -x puts the letter on the array",
			decl + `declare -x b; echo "st=$?"` + "\n" + `declare -p a`,
			"st=0\n" + `declare -ax a=([0]="p" [1]="q" [2]="r")` + "\n",
			0,
		},
		{
			// And the control that says the refusal belongs to the element
			// and not to references in general.
			"the control: a reference to a whole name is exported",
			`plain=1; typeset -n n=plain; export n=Z; echo "st=$?"` + "\n" +
				`declare -p plain`,
			"st=0\n" + `declare -x plain="Z"` + "\n",
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

// An append through a reference aimed at an element joins the element, at
// both spellings — the bare statement and the declaration's operand.
//
// Measured 2026-09-20 on bash 5.3.20 the same way. Both replaced the element
// instead of joining it, because the append's read asked for the reference's
// own name and got the empty string: the right cell was written with the
// wrong text, silently and at 0 (#3880). See Runner.storedVar.
func TestAnAppendThroughAReferenceJoinsTheElement(t *testing.T) {
	t.Parallel()
	const decl = `a=(p q r); typeset -n b='a[1]'; `
	for _, c := range []struct{ name, src, want string }{
		{"the bare statement", decl + `b+=X; echo "st=$? [${a[*]}]"`, "st=0 [p qX r]\n"},
		{"the declaration's operand", decl + `typeset b+=Y; echo "st=$? [${a[*]}]"`, "st=0 [p qY r]\n"},
		{
			"a table's key joins too",
			`typeset -A m=([k]=v); typeset -n n='m[k]'; n+=X; echo "st=$? [${m[k]}]"`,
			"st=0 [vX]\n",
		},
		{
			// The read was right all along, which is what said the append's
			// own read was the missing half rather than the resolution.
			"the control: a read of the same reference",
			decl + `echo "[$b]"`, "[q]\n",
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
