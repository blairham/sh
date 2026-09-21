// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A declaration performed **through a reference** declares the name the
// reference points at, and the binding it makes is that name's — see
// Runner.declarationThroughAReferenceShadowsTheTarget, which carries the
// rows.
//
// The redirect was never in doubt: the letters and the value do reach the
// target, which is what attributeFollowsTheReference already did. What was
// missing is the **scope**. With the shadow taken on the name the script
// wrote, the write landed on a target nothing had saved — so a `local` of
// the call's own name wrote a global that stayed written, which is a local
// leaking its value out of the call that made it (#4088).

func runNamerefRedeclared(t *testing.T, src string) (string, int) {
	t.Helper()
	sem := namerefAimSemantics()
	// A valueless declaration leaves the name unset here, so a row can ask
	// what the fresh binding holds rather than reading an empty string the
	// other answer would have written.
	sem.DeclaredNameWithoutValueIsEmpty = No
	// The name a refusal *through* a reference is spoken of under, which the
	// control rows below walk past on their way to the reference being
	// followed — it is its own subject and not this one's.
	sem.DeclarationThroughAReferenceNamesTheOperand = No
	// A valueless declaration takes the outer value out of view, which is
	// what makes the valueless row below say anything: without it the
	// target's own value shows through the fresh binding and the row cannot
	// tell a binding that was made from one that was not.
	sem.ValuelessDeclarationHidesTheOuterValue = Yes
	return runGrammar(t, src, func(d *syntax.Dialect) {
		d.ParamIndirection = true
	}, func(r *Runner) { r.Semantics = &sem })
}

func TestADeclarationThroughAReferenceMakesTheTargetsBinding(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the value is the target's, and goes away with the call",
			`f() { local -n v=g; local v=4; echo "in=[${v-U}]"; }
f
echo "out=[${g-U}]"`,
			"in=[4]\nout=[U]\n",
		},
		{
			"and the caller's own value is untouched",
			`g=SEED
f() { local -n v=g; local v=4; echo "in=[${v-U}]"; }
f
echo "out=[${g-U}]"`,
			"in=[4]\nout=[SEED]\n",
		},
		{
			"a valueless one makes the fresh binding too",
			`g=SEED
f() { local -n v=g; local v; echo "in=[${v-U}]"; }
f
echo "out=[${g-U}]"`,
			"in=[U]\nout=[SEED]\n",
		},
		{
			"a later plain assignment lands on the local target",
			`g=SEED
f() { local -n v=g; local v=4; v=9; echo "in=[${v-U}]"; }
f
echo "out=[${g-U}]"`,
			"in=[9]\nout=[SEED]\n",
		},
		{
			"and so does a removal",
			`g=SEED
f() { local -n v=g; local v=4; unset v; echo "in=[${v-U}]"; }
f
echo "out=[${g-U}]"`,
			"in=[U]\nout=[SEED]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNamerefRedeclared(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// Three controls, and each is a condition of the rule rather than a
// neighbor: a plain assignment still goes through, a second `-n` aims it
// again, and a word that makes no local binding is not a redeclaration at
// all.
func TestTheReferenceIsStillFollowedWhereNoBindingIsMade(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a plain assignment goes through the reference",
			`f() { local -n v=g; v=4; }
f
echo "out=[${g-U}]"`,
			"out=[4]\n",
		},
		{
			"a second reference letter aims it again",
			`f() { local -n v=g; local v=4; local -n v=h; v=9; }
f
echo "g=[${g-U}] h=[${h-U}]"`,
			"g=[U] h=[9]\n",
		},
		{
			"the global letter makes no binding, so it is not this",
			`f() { local -n v=g; typeset -g v=4; }
f
echo "out=[${g-U}]"`,
			"out=[4]\n",
		},
		{
			"and at the top level there is no binding to make",
			`typeset -n v=g; typeset v=4; echo "out=[${g-U}]"`,
			"out=[4]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runNamerefRedeclared(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q", out, st, tc.want)
			}
		})
	}
}

// A callee declaring a name its **caller** aimed needs nothing from this:
// the fresh cell has already dropped the reference with every other
// attribute, and the row is here so that a change to either mechanism cannot
// take the other's answer with it.
func TestACalleesDeclarationOverACallersReferenceIsTheFreshCell(t *testing.T) {
	out, st := runNamerefRedeclared(t, `outer() { local -n v=g; inner; echo "outer=[${v-U}]"; }
inner() { local v=4; echo "inner=[${v-U}]"; }
outer
echo "out=[${g-U}]"`)
	if want := "inner=[4]\nouter=[U]\nout=[U]\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}

// The reference is still **listed**, which is why the record sits beside the
// target rather than replacing it: a listing that stopped writing the row
// would be the same size of error in the other direction.
func TestTheRedeclaredReferenceIsStillListed(t *testing.T) {
	out, _ := runNamerefRedeclared(t, `g=SEED
f() { local -n v=g; local v=4; typeset -p v; }
f`)
	if !strings.Contains(out, `v='g'`) {
		t.Errorf("got %q, want the reference still in the listing", out)
	}
}
