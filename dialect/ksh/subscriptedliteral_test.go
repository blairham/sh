// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"
)

// An array literal written with subscripts creates an *association* here, and
// the `typeset -A` #1659 was filed about is that association being listed
// rather than a rule about how a sparse array is printed.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01, `/bin/ksh -c`:
//
//	a=([5]=q)        typeset -A a=([5]=q)        ${a[05]} empty, ${a[5]} is q
//	a=([05]=q)       typeset -A a=([05]=q)       ${a[05]} is q, ${a[5]} empty
//	a=([0]=x [1]=y)  typeset -A a=([0]=x [1]=y)  ${a[01]} empty
//	a[5]=q           typeset -a a=([5]=q)        ${a[05]} is q
//
// The issue read the first row as sparseness moving the letter. The third row
// is dense and still an association and the fourth is sparse and still an
// indexed array, so a rule keyed on the gap prints the wrong letter for both.
func TestASubscriptedLiteralCreatesAnAssociation(t *testing.T) {
	// The leading zero is the discriminator, and it cannot come out right by
	// accident: read as an expression `05` and `5` are one slot and both
	// answer `q`; read as keys they are two keys and one of them is empty.
	out, st := runKsh(t, t.TempDir(),
		`a=([05]=q); echo "A k05=[${a[05]}] k5=[${a[5]}] n=${#a[@]}"
typeset -p a`)
	want := "A k05=[q] k5=[] n=1\ntypeset -A a=([05]=q)\n"
	if out != want || st != 0 {
		t.Errorf("a leading-zero subscript gave %q at %d, want %q at 0", out, st, want)
	}
}

// The two shapes #1659's rule would have printed the wrong letter for.
func TestTheListingLetterFollowsTheKeysAndNotTheGap(t *testing.T) {
	out, st := runKsh(t, t.TempDir(),
		`a=([0]=x [1]=y); typeset -p a; echo "k01=[${a[01]}]"`)
	if want := "typeset -A a=([0]=x [1]=y)\nk01=[]\n"; out != want || st != 0 {
		t.Errorf("a dense literal gave %q at %d, want %q at 0", out, st, want)
	}

	out, st = runKsh(t, t.TempDir(),
		`a[5]=q; typeset -p a; echo "k05=[${a[05]}]"`)
	if want := "typeset -a a=([5]=q)\nk05=[q]\n"; out != want || st != 0 {
		t.Errorf("a sparse plain assignment gave %q at %d, want %q at 0", out, st, want)
	}
}

// The indexed letter on the *same* command is the one route back to the
// expression reading — not the attribute. Every other way of reaching a name
// that already holds an indexed array leaves the next literal's subscripts
// keys, which is what the second and third checks here pin.
func TestTheIndexedLetterBesideALiteralRestoresTheExpression(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), `typeset -a a=([1+1]=q); typeset -p a`)
	if want := "typeset -a a=([2]=q)\n"; out != want || st != 0 {
		t.Errorf("the letter beside the literal gave %q at %d, want %q at 0", out, st, want)
	}

	out, st = runKsh(t, t.TempDir(),
		`typeset -a a; a=([5]=q); typeset -p a; echo "k05=[${a[05]}]"`)
	if want := "typeset -A a=([5]=q)\nk05=[]\n"; out != want || st != 0 {
		t.Errorf("the letter on the line before gave %q at %d, want %q at 0", out, st, want)
	}

	out, st = runKsh(t, t.TempDir(), `a=(x y); a=([5]=q); typeset -p a`)
	if want := "typeset -A a=([5]=q)\n"; out != want || st != 0 {
		t.Errorf("a literal over an existing array gave %q at %d, want %q at 0", out, st, want)
	}
}
