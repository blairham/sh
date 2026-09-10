// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// Brackets with nothing between them are named and then answered: the
// subscript is reported, the operand is a flat zero, and the script carries on
// at status 0. Measured against 5.3.15 and 3.2.57 (2026-09-10), which agree.
//
// It does not depend on the name — declared, undeclared, a table, an array and
// a scalar all report alike — which is what separates this shell's answer from
// zsh's, where the same text is silent on a name that is not there and fatal
// on one that is.
//
// The real shell writes the sentence *twice* for one subscript, in both
// builds; that is an artifact of evaluating the word twice rather than a fact
// about the construct, and once is what this writes.
func TestAnEmptyArithmeticSubscriptIsReportedAndZero(t *testing.T) {
	for _, src := range []string{
		`echo $(( m[] )); echo after`,
		`w=; echo $(( m[$w] )); echo after`,
		`declare -A m; m[k]=3; echo $(( m[] )); echo after`,
		`declare -a a=(5 6 7); echo $(( a[] )); echo after`,
		`s=hello; echo $(( s[] )); echo after`,
	} {
		out, st := runBash(t, t.TempDir(), src)
		if st != 0 {
			t.Errorf("%s = %q (status %d), want 0", src, out, st)
		}
		if want := "0\nafter\n"; len(out) < len(want) || out[len(out)-len(want):] != want {
			t.Errorf("%s = %q, want a reported zero and the script running on", src, out)
		}
	}
	// The sentence, and the value that is a flat zero rather than the element
	// the subscript would have named — which is the row that tells this
	// answer from ksh93's.
	out, st := runBash(t, t.TempDir(), `declare -a a=(5 6 7); echo $(( a[] ))`)
	want := "bash: line 1: a[]: bad array subscript\n0\n"
	if out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q at 0", out, st, want)
	}
}
