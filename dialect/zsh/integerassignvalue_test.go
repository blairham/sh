// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A plain `=` inside arithmetic has the value the target's numeric attribute
// makes of it, so a float assigned to an `integer` name is the truncated
// integer — and the expansion renders that, not the float the right-hand side
// produced.
//
// **The discriminating read is the one that does not go through the store's
// own rendering.** The mirror of #4475's problem: there a float name held a
// number the letter was not printing, and a test that read the value back
// through the letter compared the rendering with itself; here the name holds
// a *rendering* — `16#6C` under `-i16`, `3.142` under `-F3` — and a rule that
// said "the value is what was stored" would produce those characters. It
// produces the **number** instead, and the two rows that say so hold the
// numeric type fixed and move only the rendering.
//
// Measured 2026-09-26 against `/opt/homebrew/bin/zsh` — zsh 5.9.2
// (aarch64-apple-darwin25.4.0) — run `-f` with no startup files. `go version
// -m` says *not a Go executable* for it. See interp/arith.go, at
// numericAttribute, and dialect/ksh/integerassignvalue_test.go for the same
// rule in the other shell that has the attribute.
func TestAnAssignmentsValueIsWhatTheAttributeMadeOfIt(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The reduction from #4595, with its own controls beside it: the
			// parameter afterwards, and the same expression unassigned.
			name: "the float that renders as an integer",
			src: `integer i; float f=3.1415; print "A=$(( i = f * 10000 ))"; print "B=$i"; ` +
				`print "C=$(( f * 10000 ))"`,
			want: "A=31415\nB=31415\nC=31415.\n",
		},
		{
			name: "a float literal assigned to an integer name",
			src:  `integer i; print $(( i = 2.0 ))`,
			want: "2\n",
		},
		{
			// The control the row above needs: an integer expression through
			// the same operator into the same kind of name.
			name: "an integer assigned to an integer name is unmoved",
			src:  `integer j; print $(( j = 7 ))`,
			want: "7\n",
		},
		// Truncation is toward zero and not toward the lower number, which
		// only the negatives can say. `-1.5` is `-1` and not `-2`.
		{"truncation, up", `integer i; print $(( i = 1.9 ))`, "1\n"},
		{"truncation, a half", `integer i; print $(( i = 1.5 ))`, "1\n"},
		{"truncation, a negative half", `integer i; print $(( i = -1.5 ))`, "-1\n"},
		{"truncation, a negative", `integer i; print $(( i = -1.9 ))`, "-1\n"},
		{"truncation, toward zero from below", `integer i; print $(( i = -0.5 ))`, "0\n"},
		{
			// **The number and not the text.** The name stores the six
			// characters `16#6C` and the assignment is worth 108.
			name: "an integer name with a base renders the base and is worth the number",
			src:  `typeset -i16 a; print "$(( a = 108 ))"; print "$a"`,
			want: "108\n16#6C\n",
		},
		{
			// Its pair, on the other side of the same question: the float
			// letter renders four characters and the assignment is worth
			// every digit. Both rows hold the numeric type fixed and move
			// the rendering, and neither answer moves with it.
			name: "a float name with a precision renders it and is worth the number",
			src:  `typeset -F3 g; print "$(( g = 3.14159265358979 ))"; print "$g"`,
			want: "3.14159265358979\n3.142\n",
		},
		{
			name: "and the exponent letter, the same",
			src:  `typeset -E3 h; print "$(( h = 3.14159265358979 ))"; print "$h"`,
			want: "3.14159265358979\n3.14e+00\n",
		},
		{
			// Moving the *type* is what moves the answer, which is what says
			// the rule is keyed on it: the same right-hand side, three kinds
			// of name.
			name: "the type is what decides",
			src: `integer i; float f; print "$(( i = 1.5 ))"; print "$(( f = 1.5 ))"; ` +
				`print "$(( u = 1.5 ))"`,
			want: "1\n1.5\n1.5\n",
		},
		{
			// The mirror of the issue's own case, which its measurement did
			// not cover: an integer expression into a float name is a float.
			name: "an integer assigned to a float name",
			src:  `float p; print $(( p = 7 ))`,
			want: "7.\n",
		},
		{
			// **Only the plain `=`.** The compound forms hand back what they
			// computed, and the parameter still holds the truncated integer
			// — so this pair holds the attribute fixed and moves the
			// operator. A rule stated about "an assignment" fails here.
			name: "a compound assignment is worth what it computed",
			src:  `integer b=1; print "$(( b += 0.5 ))"; print "$b"`,
			want: "1.5\n1\n",
		},
		{
			// The same arithmetic written out long, which is the plain
			// operator again and answers 1.
			name: "and the same sum written through the plain operator",
			src:  `integer a=1; print "$(( a = a + 0.5 ))"; print "$a"`,
			want: "1\n1\n",
		},
		{
			name: "a multiplying compound, the same way",
			src:  `integer p=1; print "$(( p *= 2.7 ))"; print "$p"`,
			want: "2.7000000000000002\n2\n",
		},
		{
			// The value reaches the expression around it and not only the
			// expansion, which is what makes this the assignment's value
			// rather than a rendering rule.
			name: "the value flows into the expression around it",
			src:  `integer i; print $(( (i = 1.5) + 0.25 ))`,
			want: "1.25\n",
		},
		{
			name: "and through a multiplication",
			src:  `integer i; print $(( (i = 2.9) * 2 ))`,
			want: "4\n",
		},
		{
			// A chain assigns rightward and every link converts, so the
			// whole of it is the integer.
			name: "a chained assignment",
			src:  `integer g1 g2; print "$(( g1 = g2 = 1.5 ))"; print "$g1 $g2"`,
			want: "1\n1 1\n",
		},
		{
			// The attribute belongs to the *name* and reaches an element of
			// it, which a rule written about a plain name would miss.
			name: "an element of an integer name",
			src:  `typeset -i b; print $(( b[2] = 1.5 ))`,
			want: "1\n",
		},
		// A float the word cannot hold saturates, and a float that is no
		// number at all is zero — the conversion an integer context already
		// makes, reached here because the attribute asks for one.
		{"too large for the word", `integer h; print $(( h = 1e30 ))`, "9223372036854775807\n"},
		{"too small for the word", `integer h; print $(( h = -1e30 ))`, "-9223372036854775808\n"},
		{"not a number at all", `integer h; print $(( h = 0.0/0.0 ))`, "0\n"},
		{
			// The store is unmoved by all of it, which is the half that was
			// already right: `${#i}`, a listing and a child all read the
			// characters.
			name: "the store is still the rendering",
			src:  `integer k; (( k = 1.5 )); print "${#k}"; typeset -p k`,
			want: "1\ntypeset -i k=1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// And the *truth* of `(( ))` is that same value, so an assignment whose
// converted number is zero is a false command.
//
// The sharpest row in the set, because it cannot be produced by any rule about
// rendering: nothing is printed at all. Measured on zsh 5.9.2, 2026-09-26.
func TestTheTruthOfAnAssignmentIsTheConvertedNumber(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			name: "a fraction stored in an integer name is false",
			src:  `integer i; (( i = 0.5 )); print "status=$? i=$i"`,
			want: "status=1 i=0\n",
		},
		{
			name: "and one that truncates to a number is true",
			src:  `integer i; (( i = 1.5 )); print "status=$? i=$i"`,
			want: "status=0 i=1\n",
		},
		{
			name: "let asks the same question",
			src:  `integer j; let "j = 0.5"; print "status=$? j=$j"`,
			want: "status=1 j=0\n",
		},
		{
			// The control: the same fraction into a float name is true,
			// because nothing converted it.
			name: "a float name is unmoved",
			src:  `float g; (( g = 0.5 )); print "status=$?"`,
			want: "status=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
