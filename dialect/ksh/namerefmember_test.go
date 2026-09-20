// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A name reference may be aimed at a **compound variable's member**, which is
// the third shape a target takes here and the one this shell refused outright.
//
// `typeset zz=(x=0); typeset -n c=zz.b` was `typeset: zz.b: invalid variable
// name` and the script ended, over a declaration ksh93u+ takes in silence — so
// a script using the construct did not run at all. The narrowest row is the
// member that **already exists**: `typeset -n c=zz.x` was refused the same
// way, which is what says this is the declaration's name check and nothing
// about creating a member.
//
// Measured 2026-09-20 against AT&T ksh93u+ 2012-08-01 (`/bin/ksh` here),
// script files under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on the null device. Compound variables are one column's construct — bash,
// zsh, dash and BusyBox ash have no parenthesized body of assignments — so
// this is a fact about ksh93 and not an axis. See interp/namerefmember.go.
func TestAReferenceIsAimedAtACompoundMember(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a member that already exists reads through the reference",
			`typeset zz=(x=0); typeset -n c=zz.x; print -r -- "[${c}]"`,
			"[0]\n",
		},
		{
			"a member that does not exist yet is still a target",
			`typeset zz=(x=0); typeset -n c=zz.b; print -r -- "[${c}]after"`,
			"[]after\n",
		},
		{
			"a write through the reference lands on the member",
			`typeset zz=(x=0); typeset -n c=zz.x; c=9; typeset -p zz`,
			"typeset -C zz=(x=9)\n",
		},
		{
			"and creates the member the reference named",
			`typeset zz=(x=0); typeset -n c=zz.b; c=7; typeset -p zz`,
			"typeset -C zz=(b=7;x=0)\n",
		},
		{
			"the reference lists as the path the script wrote",
			`typeset zz=(x=0); typeset -n c=zz.b; typeset -p c`,
			"typeset -n c=zz.b\n",
		},
		// The parent is looked for and nothing else is: every kind of name
		// stands as one, and only the **first** segment is looked for.
		{
			"a scalar is a parent",
			`zz=1; typeset -n c=zz.b; print -r -- taken`,
			"taken\n",
		},
		{
			"a declaration with no value is a parent",
			`typeset zz; typeset -n c=zz.b; print -r -- taken`,
			"taken\n",
		},
		{
			"an indexed array is a parent",
			`zz=(1 2); typeset -n c=zz.b; print -r -- taken`,
			"taken\n",
		},
		{
			"a table is a parent",
			`typeset -A zz; typeset -n c=zz.b; print -r -- taken`,
			"taken\n",
		},
		{
			"only the base of the path is looked for",
			`typeset zz=(x=0); typeset -n c=zz.b.q; print -r -- taken`,
			"taken\n",
		},
		// The other two routes that aim a reference, which read the same
		// check and moved with it. Both were the refusal before.
		{
			"an assignment aims a reference at a member",
			`typeset zz=(x=0); typeset -n c; c=zz.x; print -r -- "[${c}]"`,
			"[0]\n",
		},
		{
			"a value already standing under the name aims one",
			`typeset zz=(x=0); c=zz.x; typeset -n c; print -r -- "[${c}]"`,
			"[0]\n",
		},
		{
			"a loop re-points a reference at a member",
			`typeset zz=(x=5); typeset -n w; for w in zz.x; do print -r -- "[$w]"; done`,
			"[5]\n",
		},
		// The controls: the two shapes that were already targets must still
		// be, and a name with no dot in it must not go near any of this.
		{
			"the control: a plain name",
			`zz=1; typeset -n c=zz; print -r -- "[${c}]"`,
			"[1]\n",
		},
		{
			"the control: a name with a subscript",
			`a=(p q); typeset -n e='a[1]'; print -r -- "[${e}]"`,
			"[q]\n",
		},
		// The whole of the issue's case, which needs the store to follow
		// the reference as well as the declaration to let it be aimed —
		// #3914 landed that half in #3944, and this row is the two
		// together. It is the line a script would write.
		{
			"a compound body through a reference aimed at a member",
			"typeset zz=(x=0)\ntypeset -n c=zz.b\ntypeset c=(y=2)\n" +
				`print -r -- "zz.b.y=[${zz.b.y}]"`,
			"zz.b.y=[2]\n",
		},
		{
			"and the target lists with the member the body made",
			"typeset zz=(x=0)\ntypeset -n c=zz.b\ntypeset c=(y=2)\ntypeset -p zz",
			"typeset -C zz=(b=(y=2;)x=0)\n",
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

// The two refusals a member path can earn, which are worded differently: the
// shape is settled first and the parent second.
//
// Both end the script, which is this shell's answer to a declaration's bad
// name and not something the member path adds — see
// Semantics.BadNameToDeclarationFatal. Measured on the same run as above:
// `typeset -n c=qq.b` is `typeset: qq.b: no parent` where `typeset -n c=1a.b`
// two characters away is `typeset: 1a.b: invalid variable name`.
//
// The `no parent` half is what keeps the change honest. Admitting a member
// path without it would turn a refusal that was only mis-worded into a silent
// acceptance of a target that points nowhere.
func TestAMemberPathIsRefusedForItsShapeAndForItsParentSeparately(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"no parent at all",
			`typeset -n c=qq.b`,
			"sh: typeset: qq.b: no parent\n",
		},
		{
			"a base that is no name",
			`typeset zz=(x=0); typeset -n c=1a.b`,
			"sh: typeset: 1a.b: invalid variable name\n",
		},
		{
			"a member that is no name",
			`typeset zz=(x=0); typeset -n c=zz.b-q`,
			"sh: typeset: zz.b-q: invalid variable name\n",
		},
		{
			"a member beginning with a digit",
			`typeset zz=(x=0); typeset -n c=zz.1b`,
			"sh: typeset: zz.1b: invalid variable name\n",
		},
		{
			"the shape is settled before the parent is looked for",
			`typeset -n c=1a.b`,
			"sh: typeset: 1a.b: invalid variable name\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, status := answersRun(t, c.src+"\nprint -r -- after")
			if out != c.want || status != 1 {
				t.Errorf("wrote %q at %d, want %q at 1 with the script ended",
					out, status, c.want)
			}
		})
	}
}

// Two shapes this rule deliberately does not reach, pinned at what this shell
// answers so that a later change to either is one somebody made on purpose.
// Neither is ksh93u+'s answer.
//
//   - A **subscript on the member**: `typeset -n c='zz.x[1]'` reads the
//     element there and nothing here. The path is admitted — the declaration
//     is taken — and it is the read that does not follow the subscript
//     through a reference whose base is a member.
//   - A parent the script **unset again**. ksh93u+ keeps a node for a name
//     that was ever written and `unset` does not take it away, so `zz=1;
//     unset zz; typeset -n c=zz.b` is taken there — while `unset qq` over a
//     name nothing ever wrote is still `no parent`, which is what says it is
//     the node and not the `unset`. This shell has no record of a name it
//     once held and nothing else wants one, so the row keeps the refusal it
//     already made: the noun changed from `invalid variable name` to `no
//     parent` and the shape did not.
func TestWhatAimingAtAMemberDoesNotReach(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a subscript on the member the reference names",
			`typeset zz=(x=(1 2)); typeset -n c='zz.x[1]'; print -r -- "[${c}]"`,
			"[]\n",
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
	// The `no parent` sentence is written at the **declaration** and nowhere
	// else. ksh93u+ refuses the other aiming routes too, and in a different
	// location form — `typeset -n c=qq.b` is `t.sh[1]: typeset: qq.b: no
	// parent` against `typeset -n c; c=qq.x`'s `t.sh: line 1: qq.x: no
	// parent` — so the two are not one sentence in two places and the
	// location form is its own question. Those routes keep the refusal they
	// already made: the wrong noun at the right status, unchanged by this.
	for _, c := range []struct{ name, src, want string }{
		{
			"an assignment aiming at a parentless member",
			`typeset -n c; c=qq.x`,
			"sh: `qq.x': not a valid identifier\n",
		},
		{
			"a parentless member value adopted by the valueless form",
			`c=qq.x; typeset -n c`,
			"sh: typeset: qq.x: invalid variable name\n",
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
	// The parent a script unset again, which is a refusal rather than an
	// answer and so is pinned on its own.
	t.Run("a parent the script unset again", func(t *testing.T) {
		t.Parallel()
		out, status := answersRun(t, "zz=1; unset zz; typeset -n c=zz.b\nprint -r -- after")
		if want := "sh: typeset: zz.b: no parent\n"; out != want || status != 1 {
			t.Errorf("wrote %q at %d, want %q at 1", out, status, want)
		}
	})
}
