// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/interp"
)

// `typeset +r` does not take the readonly attribute off here: the plus form
// is refused with the builtin named, reports 1 and leaves the freeze
// standing. Measured 2026-09-07 on bash 5.3.15, `env -i PATH=/usr/bin:/bin`
// with a scratch HOME and HISTFILE, over a script file — and the same in bash
// 3.2.57 and under argv[0] of `sh` (#1168).
//
// This engine printed *nothing* here, where bash prints two lines: one for
// the refused declaration and one for the assignment behind it.
func TestTheReadonlyAttributeWillNotComeOffHere(t *testing.T) {
	if got := bash.Semantics().ReadonlyAttributeCanBeRemoved; got != interp.No {
		t.Errorf("ReadonlyAttributeCanBeRemoved = %v, want No", got)
	}
	dir := t.TempDir()
	out, st := runBash(t, dir, `typeset -r s1=1
typeset +r s1
echo "A d=$?"
s1=9
echo "B st=$? s1=[$s1]"
echo after`)
	want := "bash: line 2: typeset: s1: readonly variable\nA d=1\n" +
		"bash: line 4: s1: readonly variable\nB st=1 s1=[1]\nafter\n"
	if out != want || st != 0 {
		t.Errorf("`typeset +r` over a frozen name = %q (status %d), want %q\n"+
			"two sentences, the builtin named in the first, 1 from the builtin, and "+
			"the script carries on", out, st, want)
	}
}

// A plus form on a name nothing froze is silent and reports 0 here too, which
// is why the refusal must be asked only at a frozen name.
func TestAPlusFormOnAFreeNameIsSilentHere(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=1
typeset +r a
echo "d=$?"
a=2
echo "st=$? a=[$a]"`)
	want := "d=0\nst=0 a=[2]\n"
	if out != want || st != 0 {
		t.Errorf("`typeset +r` on a free name = %q (status %d), want %q", out, st, want)
	}
}

// A plus word beside `-r` takes nothing off, so the name is frozen: the sign
// that decides belongs to the letter and not to the word.
func TestAPlusWordBesideTheReadonlyLetterTakesNothingOffHere(t *testing.T) {
	out, _ := runBash(t, t.TempDir(), `typeset -r x=1
typeset -r +x x
echo "d=$?"
x=2
echo "st=$? x=[$x]"`)
	want := "d=0\nbash: line 4: x: readonly variable\nst=1 x=[1]\n"
	if out != want {
		t.Errorf("`typeset -r +x x` over a frozen name = %q, want %q\n"+
			"silent, 0, and still frozen — reading the word's sign would make it a "+
			"refused removal and add a sentence bash does not write", out, want)
	}
	if strings.Count(out, "readonly variable") != 1 {
		t.Errorf("out = %q, want exactly one refusal — the assignment's", out)
	}
}
