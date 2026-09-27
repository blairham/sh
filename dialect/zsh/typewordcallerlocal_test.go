// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The word a type query writes says `local` for a name a **calling** function
// made local, and not only for the frame that declared it.
//
// The three rows after the first are what make it a statement about the
// caller rather than about locals in general: the declaring frame was already
// right, a true global read from a function is plain `array` in both shells,
// and the kind does not enter into it. Without them, one row saying `local`
// would read the same as an engine that had started writing the word for
// everything.
//
// Measured on zsh 5.9.2, 2026-09-27, under `-f` from a script file with a
// scratch `HOME` (#4833).
func TestACallersLocalDescribesAsLocalFromACallee(t *testing.T) {
	out, st := answersRun(t, `inner() { print "  deep=${(t)ht}" }
outer() { inner }
top()   { typeset -a ht=(top level); outer }
top
decl()  { typeset -a dh=(x y); print "  decl=${(t)dh}" }
decl
gl=(g l)
rg()    { print "  glob=${(t)gl}" }
rg
sinner() { print "  sc=${(t)s}" }
stop()   { local s=x; sinner }
stop`)
	want := "  deep=array-local\n  decl=array-local\n  glob=array\n  sc=scalar-local\n"
	if out != want || st != 0 {
		t.Errorf("a caller's local = %q (status %d), want %q", out, st, want)
	}
}

// And a **listing** is deliberately left answering something else, which is
// the row that says the two questions are not one.
//
// In the same callee `typeset -p ht` writes `typeset -g -a ht=( top level )`
// in the reference — so that shell's own two answers disagree about one
// binding, and a fix keyed on making them agree would have made the listing
// wrong. It is pinned here rather than left to be rediscovered.
func TestAListingStillCallsACallersLocalGlobal(t *testing.T) {
	out, st := answersRun(t, `pinner() { typeset -p ht }
ptop()   { typeset -a ht=(top level); pinner }
ptop`)
	want := "typeset -g -a ht=( top level )\n"
	if out != want || st != 0 {
		t.Errorf("the listing = %q (status %d), want %q", out, st, want)
	}
}

// A private an *enclosing* call declared is the exception, and it is the one
// that decides the shape of the walk: the callee is looking past the private
// at whatever it displaced, so the word describes that and not the private.
//
// The second row is why the walk resumes outside the holder rather than
// stopping there. What a private displaced may itself be a caller's local,
// and it is then local like any other — a rule that simply stopped at the
// seal would write `scalar` for it and agree with the reference on the first
// row for the wrong reason.
//
// Measured on zsh 5.9.2, 2026-09-27 (#4833).
func TestACalleeLooksPastAPrivateWhenItWritesTheWord(t *testing.T) {
	out, st := answersRun(t, `zmodload zsh/param/private
v=9
g() { print "  outer=${(t)v} [$v]" }
f() { private v=1; g }
f
a3() { print "  nested=${(t)w3} [$w3]" }
b3() { private w3=2; a3 }
c3() { local w3=1; b3 }
c3
private tp=1
print "  top=${(t)tp}"`)
	want := "  outer=scalar [9]\n  nested=scalar-local [1]\n  top=scalar\n"
	if out != want || st != 0 {
		t.Errorf("past a private = %q (status %d), want %q", out, st, want)
	}
}
