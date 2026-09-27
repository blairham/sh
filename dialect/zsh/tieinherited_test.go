// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// Whether a local inherits the tie the name it shadows was half of, which
// this shell answers two different ways depending on **who made the tie** —
// see Runner.tieDescribesTheBinding, and tie.special for the field the two
// readings come out of (#4854).
//
// Measured 2026-09-27 on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f` from a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with a scratch HOME.
//
// The built-in rows are the controls and they are what make this a statement
// about *where the tie came from*: a rule keyed on "a local over a tie is not
// tied" passes every script row below and gets both of those wrong.
func TestALocalInheritsATieOnlyWhereTheShellMadeIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"a script's tie, shadowed by a plain local",
			`typeset -T TT tt; f(){ typeset tt; print "t=${(t)tt}" }; f`,
			"t=scalar-local\n",
		},
		{
			"a script's tie, under the `local` spelling",
			`typeset -T TT tt; f(){ local tt; print "t=${(t)tt}" }; f`,
			"t=scalar-local\n",
		},
		{
			"a script's tie, with a kind letter on the local",
			`typeset -T TT tt; f(){ typeset -a tt; print "t=${(t)tt}" }; f`,
			"t=array-local\n",
		},
		{
			"a script's tie, shadowing the scalar half",
			`typeset -T TT tt; f(){ typeset TT; print "t=${(t)TT}" }; f`,
			"t=scalar-local\n",
		},
		{
			"the shell's own tie, from the array end",
			`f(){ typeset path; print "t=${(t)path}" }; f`,
			"t=array-local-tied-special\n",
		},
		{
			"the shell's own tie, from the scalar end",
			`f(){ typeset PATH; print "t=${(t)PATH}" }; f`,
			"t=scalar-local-tied-special\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// The word is asked of the **half**, not of the pair, and this is the row
// that says so: in the same call, under the same shadowed pair, the name
// nothing stands over is still tied.
//
// It is the row that decides how the rule can be written. The predicate the
// *values* come out of answers for the tie as a whole — and rightly, since
// what an assignment moves is a property of the pair — so answering this with
// that one would take the tie off both names at once.
func TestTheUnshadowedHalfOfAScriptsTieIsStillTied(t *testing.T) {
	out, st := answersRun(t, `typeset -T TT tt
f(){ typeset TT; print "shadowed=${(t)TT} partner=${(t)tt}" }
f`)
	want := "shadowed=scalar-local partner=array-tied\n"
	if out != want || st != 0 {
		t.Errorf("a shadowed half beside its partner = %q status %d, want %q", out, st, want)
	}
}

// A tie's own declaration does not turn it off. `typeset -T` shadows both
// halves before recording the pair, so a rule asking "is this half shadowed
// anywhere" answers yes for every function-local tie there is — the depth the
// tie was made at is what keeps that from happening.
func TestATieMadeInAFunctionIsTiedInThatFunction(t *testing.T) {
	out, st := answersRun(t, `f(){ typeset -T TT tt; print "s=${(t)TT} a=${(t)tt}" }; f`)
	want := "s=scalar-local-tied a=array-local-tied\n"
	if out != want || st != 0 {
		t.Errorf("a tie in its own function = %q status %d, want %q", out, st, want)
	}
}

// And the scope the shadow stands in is not only the innermost one: a local
// taken by a *caller* is the binding a deeper frame reads, so the tie is off
// there too.
func TestACallersLocalOverAScriptsTieReachesTheCallee(t *testing.T) {
	out, st := answersRun(t, `typeset -T TT tt
g(){ print "g=${(t)tt}" }
f(){ typeset tt; g }
f`)
	want := "g=scalar-local\n"
	if out != want || st != 0 {
		t.Errorf("a callee reading a caller's local = %q status %d, want %q", out, st, want)
	}
}

// The value either side was already right and must stay so — the local starts
// empty, writing it does not move the caller's scalar, and the caller's tie is
// intact at the return. A fix aimed at the word must not move any of it.
func TestDetachingTheWordDoesNotMoveTheValues(t *testing.T) {
	out, st := answersRun(t, `typeset -T TT tt
TT=a:b
f(){ typeset tt; print "in=[$tt] n=$#tt"; tt=(x y); print "after=[$TT]" }
f
print "back=[$TT] t=${(t)tt}"`)
	want := "in=[] n=0\nafter=[a:b]\nback=[a:b] t=array-tied\n"
	if out != want || st != 0 {
		t.Errorf("the values around a detached word = %q status %d, want %q", out, st, want)
	}
}
