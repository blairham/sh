// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A scalar assigned over a name holding an array writes the array's first
// element and leaves the rest standing, as bash does — the answer opposite
// zsh's on Semantics.ScalarAssignedOverACompoundReplacesTheName.
//
// Measured against ksh93u+ (2026-09-09): `a=(1 2 3); a=x` lists as
// `typeset -a a=(x 2 3)`, and the table spelling as
// `typeset -A m=([0]=x [k]=v)`.
func TestAScalarAssignedOverAnArrayWritesTheFirstElement(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `a=(1 2 3)
a=x
print -r -- "assign: [$a] n=${#a[@]}"
typeset -p a
typeset -A m
m[k]=v
m=z
typeset -p m`)
	want := "assign: [x] n=3\ntypeset -a a=(x 2 3)\ntypeset -A m=([0]=z [k]=v)\n"
	if out != want || st != 0 {
		t.Errorf("scalar over a compound = %q (status %d), want %q", out, st, want)
	}
}

// And the loop variable of a `for`, which reached the store by a route that
// knew nothing about the rule and read the array back on every pass (#1645).
func TestAForLoopWritesTheFirstElementOfItsVariablesArray(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `v=(a b c)
for v in x y z; do print -rn -- "[$v]"; done
print
typeset -p v`)
	want := "[x][y][z]\ntypeset -a v=(z b c)\n"
	if out != want || st != 0 {
		t.Errorf("for over an array name = %q (status %d), want %q", out, st, want)
	}
}
