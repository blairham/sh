// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A **compound variable's body** assigned through a reference is stored under
// the name the reference points at, and a **member path** whose base is a
// reference is a member of that same name.
//
// The two are one rule seen from the store and from the read, and they have to
// land together: with the body following the reference and the member path not,
// `${c.a}` would read nothing at all — which is the row that made the store
// going astray invisible in practice.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here), script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device. Every row below was silently wrong at status 0: the compound
// was created under the reference's own name, so `typeset -p zz` reported
// nothing and a script noticed only when something read `zz` directly.
//
// A compound variable is one column's construct — bash, zsh, dash and BusyBox
// ash have no parenthesized body of assignments — so this is a fact about
// ksh93 and not an axis. See Runner.namerefCompoundBodyTarget.
func TestACompoundBodyThroughAReferenceLandsOnTheNameItPointsAt(t *testing.T) {
	t.Parallel()
	const ref = `typeset -n c=zz; `
	for _, c := range []struct{ name, src, want string }{
		{
			"the declaration word",
			ref + `typeset c=(a=1); typeset -p zz`,
			"typeset -C zz=(a=1)\n",
		},
		{
			"and with no declaration word at all",
			ref + `c=(a=1); typeset -p zz`,
			"typeset -C zz=(a=1)\n",
		},
		{
			"appending keeps what the first body left",
			ref + `typeset c=(a=1); c+=(b=2); print -r -- "${zz.a}${zz.b}"`,
			"12\n",
		},
		{
			"a nested body reaches the far name too",
			ref + `typeset c=(a=1 b=(y=2)); print -r -- "${zz.b.y}"`,
			"2\n",
		},
		{
			"a declaration inside the body declares the target's member",
			ref + `typeset c=(typeset -i n=5); print -r -- "${zz.n}"`,
			"5\n",
		},
		{
			"a second body replaces the first, at the target",
			ref + `typeset c=(a=1); typeset c=(d=3); print -r -- "[${zz.a}][${zz.d}]"`,
			"[][3]\n",
		},
		{
			"a chain of references is followed to its end",
			`typeset -n d=zz; typeset -n c=d; typeset c=(a=1); print -r -- "${zz.a}"`,
			"1\n",
		},
		// The reading half, which is the same rule at the member path. These
		// fail without it even though every store above lands correctly.
		{
			"a member read through the reference",
			`typeset zz=(a=1 b=2); ` + ref + `print -r -- "[${c.a}][${c.b}]"`,
			"[1][2]\n",
		},
		{
			"a member written through the reference lands on the target",
			`typeset zz=(a=1); ` + ref + `c.a=99; print -r -- "[${zz.a}][${c.a}]"`,
			"[99][99]\n",
		},
		// The controls, which are why this is one operand shape and not
		// "namerefs are broken". Both were already right and must stay right.
		{
			"the control: a scalar through a reference",
			ref + `typeset c=plain; print -r -- "${zz}"`,
			"plain\n",
		},
		{
			"the control: an array literal through a reference",
			ref + `typeset c=(1 2); print -r -- "${zz[@]}"`,
			"1 2\n",
		},
		// One call at each funnel covers every operator, because a member
		// path is a plain name by the time it gets there. A row per operator,
		// because "it is a plain name" is the claim and a list that checked
		// one would pass for a version that had quietly stopped being true of
		// the others.
		{
			"a member path takes every operator with it",
			`typeset zz=(a=1 b=2); ` + ref +
				`print -r -- "[${c.a:-D}][${c.nope:-D}][${#c.a}][${c.a+SET}][${c.nope+SET}]"`,
			"[1][D][1][SET][]\n",
		},
		{
			"appending to a member through the reference",
			`typeset zz=(a=1); ` + ref + `c.a+=x; print -r -- "[${zz.a}]"`,
			"[1x]\n",
		},
		{
			"unsetting a member through the reference",
			`typeset zz=(a=1 b=2); ` + ref + `unset c.a; print -r -- "[${zz.a}][${zz.b}]"`,
			"[][2]\n",
		},
		{
			"a prefix listing through the reference",
			`typeset zz=(a=1 b=2); ` + ref + `print -r -- "[${!c.@}][${!c.*}]"`,
			"[zz.a zz.b][zz.a zz.b]\n",
		},
		{
			"a nested member read through the reference",
			`typeset zz=(a=(y=7)); ` + ref + `print -r -- "[${c.a.y}]"`,
			"[7]\n",
		},
		// The invariant the listing owes: a prefix reached through a
		// reference answers exactly what the direct spelling answers. The
		// *content* of the two halves is this shell's answer to
		// Semantics.NamePrefixListingExcludesTheExactName, which disagrees
		// with ksh93u+ for a member that is itself a compound — `zz.n` is
		// listed there — and disagrees identically for both spellings, which
		// is the point of this row. See #3936.
		{
			"a prefix listing through the reference matches the direct one",
			`typeset zz=(a=1 n=(y=7 z=8)); ` + ref +
				`print -r -- "[${!c.n@}][${!zz.n@}]"`,
			"[zz.n.y zz.n.z][zz.n.y zz.n.z]\n",
		},
		// The control for the prefix listing: a prefix with no dot in it is
		// not a member path and must not be rewritten. It must not move.
		{
			"the control: a prefix listing on a plain name",
			`foo=1; foobar=2; print -r -- "[${!foo@}]"`,
			"[foobar]\n",
		},
		// And the one route measured and deliberately not taken, pinned at
		// what this shell answers. ksh93u+ leaves `HI` — the letter lands on
		// the target's member there and on a name called `c.b` here.
		{
			"not reached: an attribute letter over a member path",
			`typeset zz=(a=1 b=2); ` + ref + `typeset -u c.b; c.b=hi; print -r -- "[${zz.b}]"`,
			"[hi]\n",
		},
		// A member path through a reference aimed at an **element**, which
		// the read half does follow: a member of the compound an element
		// holds is the plain name `a[1].x`, so the join is the whole of it.
		{
			"a member read through a reference aimed at an element",
			`a=(p q); a[1]=(x=1); typeset -n e='a[1]'; print -r -- "[${e.x}]"`,
			"[1]\n",
		},
		{
			"and written through one",
			`a=(p q); a[1]=(x=1); typeset -n e='a[1]'; e.x=9; print -r -- "[${a[1].x}]"`,
			"[9]\n",
		},
		{
			"a table's element likewise",
			`typeset -A m; m[k]=(x=5); typeset -n t='m[k]'; print -r -- "[${t.x}]"`,
			"[5]\n",
		},
		// And the two shapes this rule does not reach. Both are **pins** and
		// not controls: no mutation of the code this change adds moves
		// either, which is checked rather than assumed. They are here to
		// record what this shell answers, so that a later change to either is
		// one somebody made on purpose. Neither is ksh93's answer — there a
		// compound body through a reference aimed at an element leaves the
		// element *holding* the compound, and one aimed at nothing refuses
		// with `no reference name` at 1.
		{
			"not reached: the element's own value, through a reference to it",
			`a=(p q r); typeset -n e='a[1]'; typeset e=(x=1); print -r -- "[${a[1]}]"`,
			"[q]\n",
		},
		{
			"reached, though: that body's members still land in the element",
			`a=(p q r); typeset -n e='a[1]'; typeset e=(x=1); print -r -- "[${a[1].x}]"`,
			"[1]\n",
		},
		{
			"not reached: a compound body through a reference aimed at nothing",
			`typeset -n u; typeset u=(a=1); print -r -- "[${u.a}]"`,
			"[1]\n",
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
