// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Which arm of an alternation a **substitution** takes, through the shell
// that prefers the one written first, against zsh 5.9.2 on 2026-09-12.
//
// The axis was the longest prefix trim's alone for a long time and the
// substitution has the same rule: it takes the longest match at each position
// exactly as `${x##pat}` does, so the arm the matcher would have preferred
// was decided before the matcher was consulted. Every line here is a pair —
// the two written orders of the same two arms — because a single value says
// nothing about a search order.
func TestASubstitutionTakesTheArmThatWasWrittenFirst(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abc
print -r -- "[${x/(a|ab)/X}][${x/(ab|a)/X}]"
print -r -- "[${x//(a|ab)/X}][${x//(ab|a)/X}]"
print -r -- "[${x/(b|bc)/X}][${x/(bc|b)/X}]"
print -r -- "[${x/#(a|ab)/X}][${x/#(ab|a)/X}]"
y=abcabc
print -r -- "[${y//(a|ab)/X}][${y//(ab|a)/X}]"`)
	want := "[Xbc][Xc]\n[Xbc][Xc]\n[aXc][aX]\n[Xbc][Xc]\n[XbcXbc][XcXc]\n"
	if out != want || st != 0 {
		t.Errorf("a substitution's arm order = %q (status %d), want %q", out, st, want)
	}
}

// An empty arm is an arm, which is the row that separates a search order from
// a second rule about length: `(|a)` replaces nothing at position 0 and
// leaves the `a` standing, where the longest reading eats it.
//
// The global spelling is the other half, and it is where a wrong fix shows
// up: the scan has to step over the unit the empty match did not take and
// carry on, so the answer is one replacement per character and not a loop.
func TestAnEmptyArmInASubstitution(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abc
print -r -- "[${x/(|a)/X}][${x/(a|)/X}]"
print -r -- "[${x//(|a)/X}][${x//(a|)/X}]"
print -r -- "[${x/(|ab)/X}][${x//(|ab)/X}]"`)
	want := "[Xabc][Xbc]\n[XaXbXc][XXbXc]\n[Xabc][XaXbXc]\n"
	if out != want || st != 0 {
		t.Errorf("an empty arm = %q (status %d), want %q", out, st, want)
	}
}

// And where a substitution has no arm to choose, measured rather than
// reasoned from the shape.
//
// `/%` pins the end of the match to the end of the value, so every match at a
// given start is the same length; `(S)` asks for the shortest match, which is
// the minimum over every arm. Both answer the same in either written order,
// which is what says the question is about the shape of the match rather than
// about the operator's name.
func TestTheSubstitutionsWithNoArmToChoose(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `x=abc
print -r -- "[${x/%(c|bc)/X}][${x/%(bc|c)/X}]"
print -r -- "[${(S)x//(a|ab)/X}][${(S)x//(ab|a)/X}]"
print -r -- "[${(S)x/(b|bc)/X}][${(S)x/(bc|b)/X}]"
print -r -- "[${x//a*b/X}][${x/(a|ab)c/X}]"`)
	want := "[aX][aX]\n[Xbc][Xbc]\n[aXc][aXc]\n[Xc][X]\n"
	if out != want || st != 0 {
		t.Errorf("a substitution with no arm to choose = %q (status %d), want %q", out, st, want)
	}
}

// The same match seen from the other side: `(#b)` and `(#m)` report the arm
// the search settled on, not the one the length reading would have taken.
//
// It is worth a test of its own because the report is filled by a *second*
// match against the pattern the script wrote, after the arm reading has moved
// the edge — so a fix that moved the edge and left the report behind would
// replace `Xbc` correctly while `$match[1]` still said `ab`.
func TestASubstitutionReportsTheArmItTook(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `setopt extendedglob
x=abc
print -r -- "[${x/(#b)(a|ab)/[$match[1]]}]"
print -r -- "[${x/(#b)(ab|a)/[$match[1]]}]"
print -r -- "[${x//(#m)(a|ab)/<$MATCH>}]"
print -r -- "[${x//(#m)(ab|a)/<$MATCH>}]"`)
	want := "[[a]bc]\n[[ab]c]\n[<a>bc]\n[<ab>c]\n"
	if out != want || st != 0 {
		t.Errorf("what a substitution reports = %q (status %d), want %q", out, st, want)
	}
}
