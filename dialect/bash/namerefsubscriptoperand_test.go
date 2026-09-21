// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A reference aimed at a **name**, with the brackets on the *operand*:
// `a=(x y); declare -n r=a; unset "r[0]"`. The reference table holds `r` and
// the operand is `r[0]`, so the rewrite that follows a reference aimed at an
// element never saw this shape and the base went to the stores as written.
//
// Measured 2026-09-21, bash 5.3.20, `env -i PATH=/usr/bin:/bin LC_ALL=C`,
// script files. Every row is the answer the operand would have got written
// with the target's own name, which is the rule: an operand's base is
// followed through the reference before either half of it is read.
//
// The first row is the one that mattered and is why this is a removal of
// more than was asked for: an index of 0 over a name holding no array is a
// whole parameter's removal, so it landed on the reference, followed it, and
// took the entire array away where bash takes one element (#4071).
func TestAnOperandsSubscriptFollowsAReferenceAimedAtAName(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the element goes and the array stays",
			"a=(x y)\ndeclare -n r=a\nunset \"r[$(echo 0)]\"\ndeclare -p a\n",
			"declare -a a=([1]=\"y\")\n",
		},
		{
			"a frozen target refuses, and by its own name",
			"a=(x y)\nreadonly a\ndeclare -n r=a\nunset \"r[0]\"\necho \"st=$?\"\ndeclare -p a\n",
			"sh: line 4: unset: a: cannot unset: readonly variable\nst=1\ndeclare -ar a=([0]=\"x\" [1]=\"y\")\n",
		},
		{
			"a key reaches the association behind the reference",
			"declare -A m=([k]=v [j]=w)\ndeclare -n r=m\nunset \"r[k]\"\ndeclare -p m\n",
			"declare -A m=([j]=\"w\" )\n",
		},
		{
			"and the whole-array brackets empty the target",
			"a=(x y z)\ndeclare -n r=a\nunset \"r[@]\"\ndeclare -p a\n",
			"declare -a a=()\n",
		},
		{
			"a reference aimed at an element is not subscripted again",
			"a=(x y z)\ndeclare -n r=a[1]\nunset \"r[0]\"\necho \"st=$?\"\ndeclare -p a\n",
			"st=0\ndeclare -a a=([0]=\"x\" [1]=\"y\" [2]=\"z\")\n",
		},
		{
			"a reference aimed nowhere real removes nothing",
			"declare -n r=nope\nunset \"r[0]\"\necho \"st=$?\"\n",
			"st=0\n",
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

// The other direction: a reference aimed at an **element** whose subscript is
// live text. `i='$(echo 0)'; declare -n r="a[$i]"` stores the six characters
// `$(echo 0)` as the aim, because the single quotes kept the `$` out of the
// round that expanded the declaration's word — so the expansion is owed when
// the reference is *read*.
//
// Measured 2026-09-21, bash 5.3.20, same conditions. `RAN` on stderr and then
// `x`, where this shell ran nothing and read empty (#4071). The command
// substitution is the discriminator rather than dressing: with `i=0` the
// declaration's own word expands it and both readings agree, so a test
// without one cannot tell the two apart.
func TestAReferenceAimedAtAnElementExpandsItsSubscriptOnRead(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"an index expands, and the substitution in it runs",
			"a=(x y)\ni='$(echo RAN >&2 ; echo 0)'\ndeclare -n r=\"a[$i]\"\necho \"[$r]\"\n",
			"RAN\n[x]\n",
		},
		{
			"a key expands the same way",
			"declare -A m=([k]=v)\ni='$(echo k)'\ndeclare -n r=\"m[$i]\"\necho \"[$r]\"\n",
			"[v]\n",
		},
		{
			"the control: a literal subscript is unmoved",
			"a=(x y)\ndeclare -n r=\"a[1]\"\necho \"[$r]\"\n",
			"[y]\n",
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
