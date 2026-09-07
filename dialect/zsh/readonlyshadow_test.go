// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// A declaration inside a function meeting a name this shell has frozen.
// Measured 2026-09-06 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME, ZDOTDIR and HISTFILE, over a script file (#1159).

// The shadow is taken here, and every spelling of the declaration takes it.
// This engine refused all of them and — worse — the refusal was **fatal**,
// so a `local` of a frozen name at a prompt cost the session.
func TestADeclarationShadowsAFrozenNameHere(t *testing.T) {
	if got := zsh.Semantics().DeclarationMayShadowAReadonly; got != interp.Yes {
		t.Errorf("DeclarationMayShadowAReadonly = %v, want Yes", got)
	}
	out, st := runZsh(t, t.TempDir(), `typeset -r x=1
f() { local x=2; echo "A in=[$x]"; echo running; }
f; echo "B st=$? out=[$x]"
g() { local x; echo "C in=[${x-UNSET}]"; }
g; echo "D out=[$x]"
h() { typeset x=3; echo "E in=[$x]"; }
h; echo "F out=[$x]"
i() { local -r x=4; echo "G in=[$x]"; }
i; echo "H out=[$x]"
j() { local y=1 x=5 z=2; echo "I y=[$y] x=[$x] z=[$z]"; }
j; echo "J st=$?"
echo after`)
	// `C in=[]` rather than UNSET is DeclaredNameWithoutValueIsEmpty, this
	// shell's own answer to a different question, and it rides along here
	// because a valueless declaration is the spelling that used to shadow
	// in silence.
	want := "A in=[2]\nrunning\nB st=0 out=[1]\nC in=[]\nD out=[1]\n" +
		"E in=[3]\nF out=[1]\nG in=[4]\nH out=[1]\n" +
		"I y=[1] x=[5] z=[2]\nJ st=0\nafter\n"
	if out != want || st != 0 {
		t.Errorf("declarations over a frozen name = %q (status %d), want %q", out, st, want)
	}
}

// And the outer name is frozen again when the function returns, which is
// what a shadow that merely cleared the attribute would get wrong.
func TestTheFrozenNameComesBackFrozenHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -r x=1
f() { local x=2; }
f
x=9
echo "never"`)
	if !strings.Contains(out, "read-only variable: x") || strings.Contains(out, "never") {
		t.Errorf("an assignment after the shadow = %q (status %d), want the name "+
			"still frozen", out, st)
	}
}

// The shape `zi.zsh:2159` is written in, and the one the issue was filed
// from: a local shadow of a *produced* readonly name.
//
// It is no longer fatal, and what is left of it refuses by name — the two
// gaps behind it are an association assigned whole and `local -h`, each
// named rather than swallowed. That is the difference this makes: before,
// the session ended here.
func TestALocalOverAProducedReadonlyNameNoLongerEndsTheSession(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/parameter
f() { local builtins=(q); echo "in=[${builtins[1]}]"; }
f; echo "st=$?"
g() { local -h EPOCHSECONDS=5; echo "g ran"; }
g; echo "gst=$?"
echo after`)
	if !strings.Contains(out, "after") || st != 0 {
		t.Errorf("a local over a produced readonly = %q (status %d), want the script "+
			"to reach the end", out, st)
	}
	if strings.Contains(out, "read-only variable") {
		t.Errorf("out = %q, want no readonly refusal — the shadow is allowed here", out)
	}
	// What is left is named, not swallowed.
	for _, want := range []string{"is not implemented yet"} {
		if !strings.Contains(out, want) {
			t.Errorf("out = %q, want %q in it", out, want)
		}
	}
}
