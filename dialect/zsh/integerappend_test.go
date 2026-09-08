// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `+=` on a name declared integer, which is arithmetic here and not a join.
// Measured 2026-09-08 on zsh 5.9.2, and on bash 5.3.15, bash 3.2.57 and
// ksh93 as well — all four agree, so this is the core's rule and the test
// stands in each dialect package rather than in one. Run through the real
// tables and not a synthetic Semantics, because an interp test built on
// permissive() cannot see a dialect and has passed against a live bug twice
// (#1386, #1477).
//
// It is on the release bar: `~/.zi/bin/zi.zsh:2538` is
// `___retval+=___last_retval` under an `integer ___retval`, and joining
// there built `0___last_retval`, which the arithmetic reader then met as two
// operands with no operator between them.

// The headline, and the pair that says the *name* decides and not the
// operator: an attributed name adds, a plain one joins, and taking the
// attribute off turns the very next `+=` back into a join.
func TestIntegerAttributeMakesAppendAddHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -i a=1; a+=2; echo "A a=$a"
typeset -i c=1 d=2; c+=d; echo "B c=$c"
e=1; f=2; e+=f; echo "C e=$e"
g=1; g+=2; echo "D g=$g"
typeset -i h=1; h+=2; typeset +i h; h+=3; echo "E h=$h"
integer i=1; i+=2; echo "F i=$i"`)
	// The values and not their shape: a test that only asked whether `a` was
	// a number would pass against 12, which is exactly the wrong answer this
	// replaced.
	want := "A a=3\nB c=3\nC e=1f\nD g=12\nE h=33\nF i=3\n"
	if out != want || st != 0 {
		t.Errorf("integer += = %q (status %d), want %q", out, st, want)
	}
}

// The right-hand side is a whole expression rather than a number, and an
// empty one is zero rather than nothing.
func TestIntegerAppendEvaluatesItsRightSideHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -i a=1 b=2; a+=b+1; echo "A a=$a"
typeset -i c=1; c+=" 2 "; echo "B c=$c"
typeset -i d=1; d+="2+3"; echo "C d=$d"
typeset -i e=1; e+=nosuch; echo "D e=$e"
typeset -i f=1; f+=; echo "E f=$f"
typeset -i g; g+=2; echo "F g=$g"
typeset -i h=1; h+=$(( 3 * 2 )); echo "G h=$h"`)
	want := "A a=4\nB c=3\nC d=6\nD e=1\nE f=1\nF g=2\nG h=7\n"
	if out != want || st != 0 {
		t.Errorf("integer += right side = %q (status %d), want %q", out, st, want)
	}
}

// The sum is written back out in the name's base, and a base the append is
// told sticks to the name exactly as a plain assignment's does.
func TestIntegerAppendKeepsTheOutputBaseHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -i16 a=255; a+=1; echo "A a=$a"
typeset -i b=1; b+=0x10; echo "B b=$b"; b=5; echo "C b=$b"`)
	want := "A a=16#100\nB b=16#11\nC b=16#5\n"
	if out != want || st != 0 {
		t.Errorf("integer += base = %q (status %d), want %q", out, st, want)
	}
}

// The float letter takes the same rule, which is why the question is about
// the *value* a name holds and not about the integer letter alone.
func TestFloatAttributeMakesAppendAddHere(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `typeset -F 3 a=1.5; a+=2.25; echo "A a=$a"
typeset -F 3 b=1; b+=; echo "B b=$b"`)
	want := "A a=3.750\nB b=1.000\n"
	if out != want || st != 0 {
		t.Errorf("float += = %q (status %d), want %q", out, st, want)
	}
}
