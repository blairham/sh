// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `${!prefix@}` and `${!prefix*}` list the names *extending* the prefix here.
//
// The name that matches the prefix exactly is set, readable, and left out.
// Measured 2026-09-13 against ksh93u+ 2012-08-01 with `ab=1; abc=2; abd=3`:
// both spellings answer `abc` and `abd`, where bash 5.3.15 answers with all
// three (#2619).
func TestAPrefixListingLeavesOutTheNameThatIsThePrefix(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`zqp=1; zqp_c=2; zqp_d=3; printf "[%s]" "${!zqp@}"`, `[zqp_c][zqp_d]`},
		{`zqp=1; zqp_c=2; zqp_d=3; printf "[%s]" "${!zqp*}"`, `[zqp_c zqp_d]`},
		// Unquoted, which is the same list through the splitting rather
		// than a second rule.
		{`zqp=1; zqp_c=2; zqp_d=3; printf "[%s]" ${!zqp@}`, `[zqp_c][zqp_d]`},
		// The exact name is the *only* one that matches, so the answer is
		// nothing at all rather than the name — which is what says the
		// filter is on the name and not on how many matched.
		{`zqp=1; printf "[%s]" "${!zqp@}"`, `[]`},
		// And a prefix nothing extends is still empty rather than a
		// refusal.
		{`printf "[%s]" "${!zqnosuch@}"`, `[]`},
		// A name that only *begins* with the prefix is listed, so the rule
		// is about equality and not about the prefix having to end at a
		// separator.
		{`zqp=1; zqp_X=2; printf "[%s]" "${!zqp@}"`, `[zqp_X]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// A prefix with a **dot** in it lists the name it spells, which is the one
// shape the exclusion above does not reach.
//
// The axis was measured over plain names and is a rule about plain names.
// Measured 2026-09-20 against ksh93u+ 2012-08-01, script files under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on the null
// device, over `typeset zz=(a=1 ab=2 n=(y=7 z=8))` — every row below was
// short by the exact name here, at status 0 (#3936).
func TestAPrefixListingKeepsAMemberPathsOwnName(t *testing.T) {
	const zz = `typeset zz=(a=1 ab=2 n=(y=7 z=8)); `
	for _, c := range []struct{ src, want string }{
		// A member that is a scalar, and one that is itself a compound.
		{zz + `printf "[%s]" "${!zz.a@}"`, `[zz.a][zz.ab]`},
		{zz + `printf "[%s]" "${!zz.n@}"`, `[zz.n][zz.n.y][zz.n.z]`},
		// One link further down, where the exact name was the only match:
		// the answer was nothing at all before.
		{zz + `printf "[%s]" "${!zz.n.y@}"`, `[zz.n.y]`},
		// The `*` spelling is the same list joined, not a second rule.
		{zz + `printf "[%s]" "${!zz.a*}"`, `[zz.a zz.ab]`},
		// The controls, and they are what say the dot is the whole of the
		// split rather than "a compound is listed": the compound `zz` is
		// left out of its own listing exactly as a scalar is, and a prefix
		// ending at the dot has no exact name to hold either way.
		{zz + `printf "[%s]" "${!zz@}"`, `[zz.a][zz.ab][zz.n][zz.n.y][zz.n.z]`},
		{zz + `printf "[%s]" "${!zz.@}"`, `[zz.a][zz.ab][zz.n][zz.n.y][zz.n.z]`},
		// And a namespace, which is spelled with a dot and lands on the
		// same side of it.
		{`namespace zqns { zqx=1; }; printf "[%s]" "${!.zqns.zqx@}"`, `[.zqns.zqx]`},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s = %q status %d, want %q at 0", c.src, out, st, c.want)
		}
	}
}

// The axis and not the code: a preset that stopped holding it would answer
// bash's way with every test above still passing against a hand-written
// filter somewhere else.
func TestThePrefixListingIsAnAxis(t *testing.T) {
	if got := ksh.Semantics().NamePrefixListingExcludesTheExactName; got != interp.Yes {
		t.Errorf("NamePrefixListingExcludesTheExactName = %v, want yes", got)
	}
}
