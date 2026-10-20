// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `+=` on a name declared integer, which is arithmetic here and not a join.
// Measured 2026-09-08 on bash 5.3.15 and bash 3.2.57, and on ksh93 and zsh
// 5.9.2 as well — all four agree, so this is the core's rule and the test
// stands in each dialect package rather than in one. Run through the real
// tables and not a synthetic Semantics, because an interp test built on
// permissive() cannot see a dialect and has passed against a live bug twice
// (#1386, #1477).

// The headline, and the pair that says the *name* decides and not the
// operator: an attributed name adds, a plain one joins, and taking the
// attribute off turns the very next `+=` back into a join.
func TestIntegerAttributeMakesAppendAddHere(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `typeset -i a=1; a+=2; echo "A a=$a"
typeset -i c=1 d=2; c+=d; echo "B c=$c"
e=1; f=2; e+=f; echo "C e=$e"
g=1; g+=2; echo "D g=$g"
typeset -i h=1; h+=2; typeset +i h; h+=3; echo "E h=$h"
declare -i i=1; i+=2; echo "F i=$i"
j() { local -i k=1; k+=2; echo "G k=$k"; }; j`)
	// The values and not their shape: a test that only asked whether `a` was
	// a number would pass against 12, which is exactly the wrong answer this
	// replaced.
	want := "A a=3\nB c=3\nC e=1f\nD g=12\nE h=33\nF i=3\nG k=3\n"
	if out != want || st != 0 {
		t.Errorf("integer += = %q (status %d), want %q", out, st, want)
	}
}

// The right-hand side is a whole expression rather than a number, and an
// empty one is zero rather than nothing.
func TestIntegerAppendEvaluatesItsRightSideHere(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `typeset -i a=1 b=2; a+=b+1; echo "A a=$a"
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

// The attribute belongs to the name, so an *element* of it adds too. Its own
// test because an element reaches a different store: an append fixed only in
// the scalar path still read `12` here.
func TestIntegerAppendToAnElementAddsHere(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `typeset -i a; a[0]=1; a[0]+=2; echo "A a=${a[0]}"
typeset -ia b=(1 2); b[1]+=5; echo "B b=${b[1]}"
typeset -iA m; m[k]=1; m[k]+=2; echo "C m=${m[k]}"`)
	want := "A a=3\nB b=7\nC m=3\n"
	if out != want || st != 0 {
		t.Errorf("integer element += = %q (status %d), want %q", out, st, want)
	}
}

// What a failed append says, and the evidence that the two sides are
// evaluated apart: the complaint names `2+` alone and never mentions the `1`
// standing in front of it. We used to blame `12+`, and that wording was the
// tell for the whole bug.
func TestIntegerAppendThatWillNotParseBlamesTheRightSideHere(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `typeset -i a=1; a+=2+; echo "unreached a=$a"`)
	want := "bash: line 1: 2+: arithmetic syntax error: operand expected (error token is \"+\")\n"
	if out != want || st != 1 {
		t.Errorf("integer += that will not parse = %q (status %d), want %q at status 1", out, st, want)
	}
}
