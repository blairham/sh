// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A hide-in-scope local over one of the shell's eight built-in ties is an
// ordinary parameter, and the word a type query writes does not call it
// `tied`.
//
// All three declaration words show it, which is what says this belongs to the
// **letter** and not to `private`: the tie table is kept per name, and a
// declaration carrying `-h` makes a binding that merely happens to be spelled
// like the shell's own.
//
// Measured on zsh 5.9.2, 2026-09-27, under `-f` from a script file (#4835).
func TestAHiddenShadowOverATieIsNotTied(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f1() { typeset -h path; print "  typeset=${(t)path}" }
f2() { local   -h path; print "  local=${(t)path}" }
f3() { private -h path; print "  private=${(t)path}" }
f7() { typeset -h PATH; print "  scalarhalf=${(t)PATH}" }
f1; f2; f3; f7`)
	want := "  typeset=scalar-local-hide\n  local=scalar-local-hide\n" +
		"  private=scalar-local-hide-special\n  scalarhalf=scalar-local-hide\n"
	if out != want || st != 0 {
		t.Errorf("a hidden shadow over a tie = %q (status %d), want %q", out, st, want)
	}
}

// The controls, and the last of them is what shapes the rule.
//
// The shell's own untied name and an ordinary name were already right and
// stay right, so the fault was the tie and not the letter, the kind or the
// name being the shell's. And at the **top level** the letter is an attribute
// with no shadow to detach, so the tie is still the binding's own and the
// word still says `tied` — the same "only where a shadow stands" the freeze
// half of `-h` already records. A rule reading the attribute without asking
// whether a shadow stood would lose that row.
//
// The value either side of the word is measured here too, because it was
// already right and a fix aimed at the word must not move it: the hidden
// local starts empty and the shell's own `path` is back at the return.
func TestTheHideLetterLeavesATieAloneWithoutAShadow(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f4() { private -h HOME; print "  own=${(t)HOME}" }
f5() { private -h v;    print "  ordinary=${(t)v}" }
f4; f5
typeset -h path
print "  top=${(t)path}"
f6() { private -h path; print "  inside=[$path]" }
f6
print "  after=[$path]"`)
	want := "  own=scalar-local-hide-special\n  ordinary=scalar-local-hide-special\n" +
		"  top=array-tied-hide-special\n  inside=[]\n  after=[/usr/bin /bin]\n"
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
}
