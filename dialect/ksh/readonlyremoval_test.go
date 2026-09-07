// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/interp"
)

// `typeset +r` does not take the readonly attribute off here, and the refusal
// **ends the script** — which is a declaration's refusal doing what every
// other one of them does in this shell. Measured 2026-09-07 on ksh93u+
// 2012-08-01, `env -i PATH=/usr/bin:/bin` with a scratch HOME and HISTFILE,
// over a script file (#1168).
//
// The sentence is this shell's own arrangement and the reason
// ReadonlyRemovalNamesBuiltin exists: the plus form takes the *builtin's*
// location with the builtin named, where an assignment through the identical
// word takes the plain line form.
func TestTheReadonlyAttributeWillNotComeOffHere(t *testing.T) {
	if got := ksh.Semantics().ReadonlyAttributeCanBeRemoved; got != interp.No {
		t.Errorf("ReadonlyAttributeCanBeRemoved = %v, want No", got)
	}
	if !ksh.Diagnostics().ReadonlyRemovalNamesBuiltin["typeset"] {
		t.Error("ReadonlyRemovalNamesBuiltin has no `typeset` entry, so a refused " +
			"plus form loses the builtin's name and its location — this shell puts " +
			"both there for the plus form and neither for an assignment")
	}
	out, st := runKsh(t, t.TempDir(), `typeset -r s1=1
typeset +r s1
echo never`)
	if !strings.Contains(out, "typeset: s1: is read only") {
		t.Errorf("`typeset +r` over a frozen name = %q, want the builtin named", out)
	}
	if strings.Contains(out, "never") || st != 1 {
		t.Errorf("out = %q (status %d), want the script to end and exit 1", out, st)
	}
}

// And the same word carrying a value is refused as an *assignment* instead,
// which takes the plain line form: the two shapes are one word and two
// sentences, and the value is what tells them apart.
func TestAPlusFormWithAValueIsRefusedAsAnAssignmentHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -r s1=1
typeset +r s1=5
echo never`)
	if strings.Contains(out, "typeset: s1") {
		t.Errorf("`typeset +r s1=5` = %q, want the builtin *not* named — the "+
			"assignment is refused before the attribute is reached", out)
	}
	if !strings.Contains(out, "s1: is read only") || strings.Contains(out, "never") || st != 1 {
		t.Errorf("out = %q (status %d), want the assignment refusal and the script "+
			"to end", out, st)
	}
}

// A `typeset` in a keyword function may shadow a frozen name here, as in zsh
// — this shell has no `local` and answers all the same. It was down as a
// refusal it does not make, read off the fatality its *POSIX*-style functions
// produce, where there is no scope to shadow into at all (#1177).
func TestADeclarationShadowsAFrozenNameHere(t *testing.T) {
	if got := ksh.Semantics().DeclarationMayShadowAReadonly; got != interp.Yes {
		t.Errorf("DeclarationMayShadowAReadonly = %v, want Yes", got)
	}
	out, st := runKsh(t, t.TempDir(), `typeset -r x=1
function f { typeset x=2; echo "A in=[$x]"; echo running; }
f; echo "B st=$? out=[$x]"
function g { typeset -r y=1; echo "C in=[$y]"; }
g
y=2
echo "D st=$? y=[$y]"
echo after`)
	want := "A in=[2]\nrunning\nB st=0 out=[1]\nC in=[1]\nD st=0 y=[2]\nafter\n"
	if out != want || st != 0 {
		t.Errorf("declarations in a keyword function over a frozen name = %q "+
			"(status %d), want %q\nthe shadow stands, and the freeze a declaration "+
			"adds ends with the call", out, st, want)
	}
}

// The POSIX-style function is the other reading, and it is a different
// question: there is no scope, so the same `typeset` is an ordinary
// assignment to the frozen global and this shell ends the script over it.
func TestAParenFunctionHasNoScopeToShadowIntoHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -r x=1
f() { typeset x=2; echo "in=[$x]"; }
f
echo never`)
	if !strings.Contains(out, "x: is read only") || strings.Contains(out, "never") || st != 1 {
		t.Errorf("`typeset x=2` in a paren function over a frozen name = %q "+
			"(status %d), want the ordinary refusal and the script to end — this is "+
			"TypesetLocalNeedsKeywordFunction, not the shadow axis", out, st)
	}
}
