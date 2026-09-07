// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// `typeset +r` takes the readonly attribute back off here, and this is the
// only shell in the panel that lets it. Measured 2026-09-07 on zsh 5.9.2,
// `env -i PATH=/usr/bin:/bin` with a scratch HOME, ZDOTDIR and HISTFILE, over
// a script file (#1168).
//
// Before this the plus form did nothing and the script then met the refusal
// the *next* line was going to meet anyway — and that refusal was **fatal**,
// so undoing a `readonly` at a prompt cost the session.
func TestTheReadonlyAttributeComesOffHere(t *testing.T) {
	if got := zsh.Semantics().ReadonlyAttributeCanBeRemoved; got != interp.Yes {
		t.Errorf("ReadonlyAttributeCanBeRemoved = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `typeset -r s1=1
typeset +r s1
echo "A d=$?"
s1=9
echo "B st=$? s1=[$s1]"
typeset -r b=1
declare +r b
b=8
echo "C b=[$b] st=$?"
typeset -r v=1
typeset +r v=5
echo "D d=$? v=[$v]"
f() { local -r y=1; local +r y; y=2; echo "E in=[$y]"; }
f; echo "F st=$?"
echo after`)
	want := "A d=0\nB st=0 s1=[9]\nC b=[8] st=0\nD d=0 v=[5]\n" +
		"E in=[2]\nF st=0\nafter\n"
	if out != want || st != 0 {
		t.Errorf("the plus form over frozen names = %q (status %d), want %q\n"+
			"`typeset +r`, `declare +r`, a plus form carrying its own value, and "+
			"`local +r` over a freeze the same call made — all four leave the name "+
			"writable here", out, st, want)
	}
}

// A plus form on a name nothing froze is silent and reports 0, which is the
// shape a script writes to make sure a name is writable.
func TestAPlusFormOnAFreeNameIsSilentHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `a=1
typeset +r a
echo "d=$?"
a=2
echo "st=$? a=[$a]"`)
	want := "d=0\nst=0 a=[2]\n"
	if out != want || st != 0 {
		t.Errorf("`typeset +r` on a free name = %q (status %d), want %q", out, st, want)
	}
}

// The letter carries its own sign: a plus word beside `-r` takes nothing off,
// so the name is frozen and stays frozen.
func TestAPlusWordBesideTheReadonlyLetterTakesNothingOffHere(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `n=1
typeset -r +x n
n=2
echo "never"`)
	if !strings.Contains(out, "read-only variable: n") || strings.Contains(out, "never") {
		t.Errorf("`typeset -r +x n` = %q, want n frozen — the plus belongs to the "+
			"`x` letter beside it", out)
	}
}
