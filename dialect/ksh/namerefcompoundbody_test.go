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
		// reference answers exactly what the direct spelling answers. Both
		// halves now hold the name that *is* the prefix, which is ksh93u+'s
		// answer for a member path — see withoutTheExactName, where the
		// measurement is, and TestAPrefixListingKeepsAMemberPathsOwnName for
		// the direct spellings on their own (#3936).
		{
			"a prefix listing through the reference matches the direct one",
			`typeset zz=(a=1 n=(y=7 z=8)); ` + ref +
				`print -r -- "[${!c.n@}][${!zz.n@}]"`,
			"[zz.n zz.n.y zz.n.z][zz.n zz.n.y zz.n.z]\n",
		},
		// The control for the prefix listing: a prefix with no dot in it is
		// not a member path and must not be rewritten. It must not move.
		{
			"the control: a prefix listing on a plain name",
			`foo=1; foobar=2; print -r -- "[${!foo@}]"`,
			"[foobar]\n",
		},
		// An attribute letter over a member path, which is the same rule at
		// the declaration's **operand**: the letter used to land on a name
		// called `c.b` that nothing read, so the value reached `zz.b` plain.
		// The operand is resolved before the scope is taken, which is what
		// keeps the shadow and the letter on one cell (#3935).
		{
			"an attribute letter over a member path",
			`typeset zz=(a=1 b=2); ` + ref + `typeset -u c.b; c.b=hi; print -r -- "[${zz.b}]"`,
			"[HI]\n",
		},
		{
			"and an integer letter, which shapes the value on the way in",
			`typeset zz=(a=1 b=2); ` + ref + `typeset -i c.a; c.a=3+4; print -r -- "[${zz.a}]"`,
			"[7]\n",
		},
		{
			"the letter is recorded on the target's member, not on the path",
			`typeset zz=(a=1 b=2); ` + ref + `typeset -u c.b; typeset -p zz`,
			"typeset -C zz=(a=1;typeset -u b=2)\n",
		},
		// The control: a member path with no reference in front of it took
		// its letter already, which is what says the defect was the walk and
		// not the letter.
		{
			"the control: an attribute letter over a member of a plain compound",
			`typeset zz=(a=1 b=2); typeset -u zz.b; zz.b=hi; print -r -- "[${zz.b}]"`,
			"[HI]\n",
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
		// A reference aimed at an **element**, where the element itself comes
		// to hold the compound. The members always landed in its namespace —
		// that is the member path again, and the second row is the control
		// that says so — and the element kept the string it held before
		// (#3931).
		{
			"the element holds the compound the reference carried into it",
			`a=(p q r); typeset -n e='a[1]'; typeset e=(x=1); typeset -p a`,
			"typeset -a a=(p (x=1) r)\n",
		},
		{
			"so a read of the element is the body and not the old string",
			`a=(p q r); typeset -n e='a[1]'; typeset e=(x=1); print -r -- "[${a[1]}]"`,
			"[(\n\tx=1\n)]\n",
		},
		{
			"and that body's members are still in the element's namespace",
			`a=(p q r); typeset -n e='a[1]'; typeset e=(x=1); print -r -- "[${a[1].x}]"`,
			"[1]\n",
		},
		{
			"a table's element likewise holds it",
			`typeset -A m=([k]=v); typeset -n t='m[k]'; typeset t=(x=1); typeset -p m`,
			"typeset -A m=([k]=(x=1))\n",
		},
		// The control for that one: a **scalar** through the same reference
		// reached the element already, through setVarAs and
		// storeThroughNamerefElement. It must not move.
		{
			"the control: a scalar through a reference aimed at an element",
			`a=(p q r); typeset -n e='a[1]'; typeset e=Z; typeset -p a`,
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

// A compound variable's body written through a reference **aimed at nothing**
// is refused, where a scalar and an array literal through the same reference
// are not.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh`), script
// files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the
// null device: `typeset -n u; typeset u=(a=1)` writes `u: no reference name`
// at 1 and the script ends there, where this shell stored a compound called
// `u` and ran on at 0. Its own function rather than a row of the table above
// because the answer is a diagnostic and a status rather than a value (#3932).
//
// Separate from the table for a second reason worth keeping: the refusal is
// what the *store* makes of an unaimed reference and the read half shares
// none of it — see Runner.namerefCompoundBodyStore, where the fold is argued.
func TestACompoundBodyThroughAnUnaimedReferenceIsRefused(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"the declaration word",
			`typeset -n u; typeset u=(a=1); print -r -- after`,
			"sh: u: no reference name\n",
		},
		{
			"and with no declaration word at all",
			`typeset -n u; u=(a=1); print -r -- after`,
			"sh: u: no reference name\n",
		},
		{
			"appending is the same refusal",
			`typeset -n u; u+=(a=1); print -r -- after`,
			"sh: u: no reference name\n",
		},
		{
			"and it ends the script from inside a call",
			`f() { typeset -n u; typeset u=(a=1); }; f; print -r -- after`,
			"sh: u: no reference name\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src)
			if out != c.want || status != 1 {
				t.Errorf("wrote %q at %d, want %q at 1", out, status, c.want)
			}
		})
	}
}

// And the two controls, which are why the refusal is the compound body's and
// not "an unaimed reference is refused": a scalar through one is silent and
// an array literal through one stores under the reference's own name.
//
// Both were already right and both stay right. They are the rows that say a
// guard on the unaimed state cannot be moved up into the walk every operand
// shape shares.
func TestAnUnaimedReferenceTakesAScalarAndAnArrayLiteral(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a scalar", `typeset -n u; typeset u=plain; print -r -- "st=$? [${u}]"`, "st=0 []\n"},
		{"an array literal", `typeset -n u; typeset u=(1 2); print -r -- "st=$? [${u[@]}]"`, "st=0 [1 2]\n"},
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
