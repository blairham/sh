// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A bare `+` makes an ordinary local where the word alone makes a private,
// and **the visibility follows it**: the callee sees the binding there and
// read straight past it here.
//
// That second half is what makes this more than an attribute word. A script
// that writes the sign gets a name its callees can see in the reference and
// cannot here (#4836).
//
// Measured on zsh 5.9.2, 2026-09-27, under `-f` from a script file with the
// module loaded.
func TestABareSignLeavesAnOrdinaryLocal(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private + v; print "  word=${(t)v}" }
f
g() { local -P + w; print "  letter=${(t)w}" }
g
h() { private -a + q; print "  kind=${(t)q}" }
h`)
	want := "  word=scalar-local\n  letter=scalar-local\n  kind=array-local\n"
	if out != want || st != 0 {
		t.Errorf("a bare sign = %q (status %d), want %q", out, st, want)
	}
	out, st = answersRun(t, `zmodload zsh/param/private
w=9
g() { print "  g=[$w]" }
f() { private + w; w=1; g; print "  f=[$w]" }
f`)
	want = "  g=[1]\n  f=[1]\n"
	if out != want || st != 0 {
		t.Errorf("the visibility = %q (status %d), want %q", out, st, want)
	}
}

// The controls, and the second is the sharp one: a **value** on an operand
// and the reference keeps the private, so the sign does not mean "never
// private" — it is the valueless form that is not a declaration.
func TestTheSignIsNotARequestToNeverBePrivate(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private v;     print "  nosign=${(t)v}" }
g() { private + v=1; print "  valued=${(t)v}" }
f; g`)
	want := "  nosign=scalar-local-hide-special\n  valued=scalar-local-hide-special\n"
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
}

// And it is a decision about the **line**, not about each operand, which is
// the reading the issue proposed and the measurement killed.
//
// `private + a b=1 c` leaves all three private in the reference and a callee
// sees none of them, where `private + a b` — nothing valued anywhere — leaves
// both ordinary. So one valued operand makes the whole line a declaration
// again. Both rows are here because either alone reads as the other.
func TestOneValuedOperandMakesTheWholeLineADeclaration(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
f() { private + a b=1 c; print "  a=${(t)a} b=${(t)b} c=${(t)c}" }
f
g() { private + d e; print "  d=${(t)d} e=${(t)e}" }
g`)
	want := "  a=scalar-local-hide-special b=scalar-local-hide-special" +
		" c=scalar-local-hide-special\n  d=scalar-local e=scalar-local\n"
	if out != want || st != 0 {
		t.Errorf("a mixed line = %q (status %d), want %q", out, st, want)
	}
	// And the scope refusal reads the same decision, which is what a
	// per-operand exemption got wrong: `path` carries no value on this line
	// and is refused all the same, because the line has one.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private + path b=1; print "  st=$?" }
f
print "  after=[$path]"`)
	want = "f:private: can't change scope of existing param: path\n  st=1\n  after=[/usr/bin /bin]\n"
	if out != want || st != 0 {
		t.Errorf("the refusal on a mixed line = %q (status %d), want %q", out, st, want)
	}
	// And a `name=( … )` operand is a value for this rule too, which is the
	// row the argument scan cannot see on its own: the parser takes the
	// literal off the command line and hands the utility the bare name, so
	// every word it reads looks valueless. Measured 2026-09-27 with the rest
	// (#4855).
	out, st = answersRun(t, `zmodload zsh/param/private
g() { print "  g sees a=[$a] b=[$b] c=[$c]" }
a=A b=B c=C
f() { private + a b=(1) c; print "  a=${(t)a} b=${(t)b} c=${(t)c}"; g }
f`)
	want = "  a=scalar-local-hide-special b=array-local-hide-special" +
		" c=scalar-local-hide-special\n  g sees a=[A] b=[B] c=[C]\n"
	if out != want || st != 0 {
		t.Errorf("a literal operand on a plus line = %q (status %d), want %q", out, st, want)
	}
	// And the refusal reads it too, which is the pair that says the literal
	// counts on the *line* and not merely for its own name.
	out, st = answersRun(t, `zmodload zsh/param/private
f() { private + path b=(1); print "  st=$?" }
f
print "  after=[$path]"`)
	want = "f:private: can't change scope of existing param: path\n  st=1\n  after=[/usr/bin /bin]\n"
	if out != want || st != 0 {
		t.Errorf("the refusal beside a literal operand = %q (status %d), want %q", out, st, want)
	}
}

// The sign word does not change the sign of a letter already written, which
// is not `private`'s row at all — it is the same bug under `typeset`, and it
// is why the kind letter used to go missing from `private -a + q` too.
//
// The export row is the second spelling and the one that says this is about
// the letters rather than about the container: `typeset -x + v` is
// `scalar-export` in the reference, with no `local` in it, because the export
// letter declares a global in this shell. Both were dropped here.
//
// Bare `typeset +` is the listing the word is and is the control that keeps
// the sign meaning something: it writes names without values, so the sign is
// still read where no letter was written.
func TestASignWordKeepsTheLettersBeforeIt(t *testing.T) {
	out, st := answersRun(t, `f() { typeset -a + q; print "  array=${(t)q}" }
f
g() { typeset -x + v; print "  export=${(t)v}" }
g
h() { typeset + z; print "  bare=${(t)z}" }
h
k() { typeset -x e=1; typeset +x e; print "  off=${(t)e}" }
k`)
	want := "  array=array-local\n  export=scalar-export\n" +
		"  bare=scalar-local\n  off=scalar\n"
	if out != want || st != 0 {
		t.Errorf("a sign word after a letter = %q (status %d), want %q", out, st, want)
	}
}
