// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A scalar assigned over a name holding an array writes the array's *first*
// element and leaves the rest standing — the other side of
// Semantics.ScalarAssignedOverACompoundReplacesTheName, whose replacing answer
// is zsh's.
//
// Measured against bash 5.3.15 (2026-09-09), and the same in 3.2.57 and under
// argv[0] of `sh`. `declare -p` is the instrument: `$a` reads `x` under both
// answers, because a plain reference to an array is its first element here, so
// the value alone cannot tell them apart.
//
// The base and not the lowest subscript standing: `a=([5]=q); a=x` grows a
// first element and `q` does not move.
func TestAScalarAssignedOverAnArrayWritesTheFirstElement(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(1 2 3)
a=x
echo "assign: [$a] n=${#a[@]}"
declare -p a
declare -A m=([k]=v)
m=z
declare -p m
s=([5]=q)
s=x
declare -p s`)
	want := "assign: [x] n=3\n" +
		"declare -a a=([0]=\"x\" [1]=\"2\" [2]=\"3\")\n" +
		"declare -A m=([0]=\"z\" [k]=\"v\" )\n" +
		"declare -a s=([0]=\"x\" [5]=\"q\")\n"
	if out != want || st != 0 {
		t.Errorf("scalar over a compound = %q (status %d), want %q", out, st, want)
	}
}

// The loop variable of a `for` goes through the same rule rather than through
// one of its own. It went through neither: the array was left standing whole
// and every pass read it back unchanged (#1645).
func TestAForLoopWritesTheFirstElementOfItsVariablesArray(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `v=(a b c)
for v in x y z; do printf "[%s]" "$v"; done
echo
declare -p v`)
	want := "[x][y][z]\ndeclare -a v=([0]=\"z\" [1]=\"b\" [2]=\"c\")\n"
	if out != want || st != 0 {
		t.Errorf("for over an array name = %q (status %d), want %q", out, st, want)
	}
}

// A prefix assignment before a builtin is transient, so the array it wrote
// into is given back exactly as it was.
func TestAPrefixAssignmentGivesBackTheWholeArray(t *testing.T) {
	out, st := runBash(t, t.TempDir(), `a=(p q)
a=x true
declare -p a`)
	want := "declare -a a=([0]=\"p\" [1]=\"q\")\n"
	if out != want || st != 0 {
		t.Errorf("prefix over an array = %q (status %d), want %q", out, st, want)
	}
}
