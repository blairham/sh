// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `zle -T <transformation> <widget>` registers a function as a named
// transformation the line editor calls at a defined point, and `zle -Tr`
// takes the registration away. Measured 2026-09-26 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), run `-f` with `env -u FPATH` over a script
// file with no terminal — which is what says this is registration rather than
// drawing: it answers 0 with no editor running at all (#4450).
func TestZleTransformationRegisters(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zle
f() { print x; }
zle -T tc f;  print "A=$?"
zle -Tr tc;   print "B=$?"
zle -T -r tc; print "C=$?"
zle -Tr nosuchtype; print "D=$?"
zle -T tc nosuchfn; print "E=$?"
zle -- tc f 2>/dev/null; print "F=$?"`)
	// D and E are the two the implementation must *not* check: a removal
	// looks up no name at all, and a registration does not look up the
	// widget. F is the marker ending the letters as ever — a call of a
	// widget called `tc`, outside an editor, which is a different refusal.
	want := "A=0\nB=0\nC=0\nD=0\nE=0\nF=1\n"
	if out != want || st != 0 {
		t.Errorf("zle -T = %q (status %d), want %q", out, st, want)
	}
}

// The arity, which is the half a bare "accept the letter" gets wrong: the
// letter is refused at the option scan today, before anything looks at what
// was passed, so the fix is to accept it *and then* check its operands.
//
// The two `too many` sentences really do differ by the word `option`, and
// they are asserted whole rather than by substring for that reason.
func TestZleTransformationArity(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zle
f() { :; }
zle -T;             print "A=$?"
zle -T tc;          print "B=$?"
zle -T nosuchtype;  print "C=$?"
zle -T tc f extra;  print "D=$?"
zle -Tr;            print "E=$?"
zle -Tr tc f;       print "F=$?"`)
	// Row C is the order: the arity is judged before the name, so a single
	// unknown name is still "too few" rather than "no such transformation".
	want := "zsh:zle:3: too few arguments for option -T\nA=1\n" +
		"zsh:zle:4: too few arguments for option -T\nB=1\n" +
		"zsh:zle:5: too few arguments for option -T\nC=1\n" +
		"zsh:zle:6: too many arguments for -T\nD=1\n" +
		"zsh:zle:7: too few arguments for option -T\nE=1\n" +
		"zsh:zle:8: too many arguments for option -T\nF=1\n"
	if out != want || st != 0 {
		t.Errorf("the arity of zle -T = %q (status %d), want %q", out, st, want)
	}
}

// There is exactly one transformation name, and that is measured rather than
// read: asked for every one-, two- and three-letter name — 18,278 of them —
// this shell takes `tc` and refuses every other one.
func TestZleTransformationNameIsChecked(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zle
f() { :; }
zle -T nosuchtype f; print "A=$?"
zle -T '' f;         print "B=$?"
zle -T tc f;         print "C=$?"`)
	want := "zsh:zle:3: -T: no such transformation 'nosuchtype'\nA=1\n" +
		"zsh:zle:4: -T: no such transformation ''\nB=1\n" +
		"C=0\n"
	if out != want || st != 0 {
		t.Errorf("the transformation name = %q (status %d), want %q", out, st, want)
	}
}

// `-r` is a modifier only `-T` reads, and on its own it is the bare `zle`.
func TestZleForgetLetterIsAModifier(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `zmodload zsh/zle
f() { :; }
zle -r;        print "A=$?"
zle -N -r w f; print "B=$?"
zle -Nr w2 f;  print "C=$?"
zle -l`)
	want := "A=1\nB=0\nC=0\nw (f)\nw2 (f)\n"
	if out != want || st != 0 {
		t.Errorf("zle -r = %q (status %d), want %q", out, st, want)
	}
}
