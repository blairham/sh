// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A subscript that will not evaluate is refused even when the parameter does
// not exist, and the refusal ends the shell.
//
// The whole panel makes it — bash 5.3.20, bash 3.2.57, ksh93u+ and zsh 5.9.2
// all refuse `: ${x[1+]}` on a name nothing created, each in its own words —
// and this column was the only one of ours that skipped the expression
// instead of evaluating it. Measured 2026-09-27 from a script file (#4837).
//
// The **subshell** is how the row is visible at all: at the top level the
// reference prints its complaint and stops, so a probe comparing only stdout
// there sees two shells that both produce nothing. The boundary gives the
// caller a status to read.
func TestASubscriptThatWillNotEvaluateIsRefusedOnAnAbsentName(t *testing.T) {
	out, st := answersRun(t, `( : ${x[1+]} ); print "  sub=$?"
v=$( : ${y[1+]} ); print "  cmdsub=$?"
f(){ : ${z[1+]} }; ( f ); print "  func=$?"
eval ': ${w[1+]}'; print "  eval=$?"`)
	const bad = "bad math expression: operand expected at end of string\n"
	want := "zsh:1: " + bad + "  sub=1\n" +
		"zsh:2: " + bad + "  cmdsub=1\n" +
		"f: " + bad + "  func=1\n" +
		"(eval):1: " + bad + "  eval=1\n"
	if out != want || st != 0 {
		t.Errorf("through a boundary = %q (status %d), want %q", out, st, want)
	}
}

// The controls that locate it: put **anything at all** under the name and
// every column already agreed, so the expression was being skipped rather
// than evaluated and forgiven.
//
// Five kinds of "exists but is empty" — an array, an empty array, a
// declaration with no assignment, a scalar, an empty scalar — and only the
// name nothing created ever diverged.
func TestAnythingUnderTheNameAlreadyRefusedIt(t *testing.T) {
	out, st := answersRun(t, `( x=(a b);      : ${x[1+]} ); print "  c1=$?"
( x=();         : ${x[1+]} ); print "  c2=$?"
( typeset -a x; : ${x[1+]} ); print "  c3=$?"
( x=plain;      : ${x[1+]} ); print "  c4=$?"
( x="";         : ${x[1+]} ); print "  c5=$?"`)
	const bad = "bad math expression: operand expected at end of string\n"
	want := ""
	for i, tag := range []string{"c1", "c2", "c3", "c4", "c5"} {
		want += "zsh:" + string(rune('1'+i)) + ": " + bad + "  " + tag + "=1\n"
	}
	if out != want || st != 0 {
		t.Errorf("the controls = %q (status %d), want %q", out, st, want)
	}
}

// And the other control, which is the half a refusal added carelessly would
// break: a subscript that *does* evaluate is still nothing on an absent name,
// silently and at 0.
//
// The value side was already right and is not what was missing — `${x[1]}`
// expands to nothing in both shells — so what this pins is that the new
// refusal reaches only the expression that will not evaluate.
func TestAWorkingSubscriptOnAnAbsentNameIsStillNothing(t *testing.T) {
	out, st := answersRun(t, `( : ${x[1]} );   print "  n1=$?"
( : ${x[-1]} );  print "  n2=$?"
( : ${x[@]} );   print "  n3=$?"
( : ${x[1,2]} ); print "  n4=$?"
print "  v=[${x[1]}]"`)
	want := "  n1=0\n  n2=0\n  n3=0\n  n4=0\n  v=[]\n"
	if out != want || st != 0 {
		t.Errorf("a working subscript = %q (status %d), want %q", out, st, want)
	}
}
