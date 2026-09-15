// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `typeset -E n` — this shell's reading, which is `%.*e` with **n−1** places.
//
// Measured 2026-09-15 against zsh 5.9.2, `env -i` with a scratch HOME and no
// startup files, from a script file. The letter is a *format* and not a
// width, which is why it did not travel with `-L`, `-R` and `-Z` in #2538 —
// see interp/floatformat.go, and dialect/ksh/floatformat_test.go for the
// other reading of the same letter.
func TestTheExponentLetterIsAFormat(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"three significant figures", `typeset -E 3 a=3.14159; echo "$a"`, "3.14e+00\n"},
		// A small number keeps the same three figures and moves the
		// exponent, which is what says the number is significant figures
		// rather than places after the point.
		{"a small number", `typeset -E 3 b=0.000123456; echo "$b"`, "1.23e-04\n"},
		// And a number that needs no exponent gets one anyway, which is the
		// discriminating row against ksh93's `%g`.
		{"a number that needs no exponent", `typeset -E 3 f=100; echo "$f"`, "1.00e+02\n"},
		// One figure is no places at all — the n−1 in so many words.
		{"one figure", `typeset -E 1 g=3.14159; echo "$g"`, "3e+00\n"},
		{"the default is ten", `typeset -E c=1.5; echo "$c"`, "1.500000000e+00\n"},
		// The value is an expression, as `-F`'s is, and a word that is no
		// number at all is zero.
		{"a word that is no number", `typeset -E 3 h=abc; echo "$h"`, "0.00e+00\n"},
		{"an expression", `typeset -E 3 e=1+2; echo "$e"`, "3.00e+00\n"},
		{"attached to the letter", `typeset -E3 a=3.14159; echo "$a"`, "3.14e+00\n"},
		// An append joins as numbers, through the same rendering.
		{"an append", `typeset -E 3 a=1.5; a+=1; echo "$a"`, "2.50e+00\n"},
		// The listing writes the letter and **no width**, exactly as it
		// writes none for `-F`.
		{"the listing writes no width", `typeset -E 3 a=3.14159; typeset -p a`, "typeset -E a=3.14e+00\n"},
		{"and the plain letter stays plain", `typeset -F 3 a=1.5; typeset -p a`, "typeset -F a=1.500\n"},
		// The plus form takes the attribute off and leaves the text.
		{"the plus form", `typeset -E 3 a=1.5; typeset +E a; echo "$a"; typeset -p a`, "1.50e+00\ntypeset a=1.50e+00\n"},
		// A later integer declaration takes the float attribute off, which
		// is the rule `-F` already had and which the `E` letter has to reach
		// as well.
		{"an integer letter afterwards", `typeset -E 3 a=1.5; typeset -i a; echo "$a"`, "1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// The three questions this shell answers one way and ksh93 the other, each on
// the row that discriminates them.
func TestWhereTheNumericLettersPartFromKsh(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// A detached number reaches its letter wherever the letter
			// stands, and taking it discards the rest of the word.
			// ksh93 reads the `3` as an operand here and refuses it.
			name: "a detached number past another letter",
			src:  `typeset -El 3 a=1.5; echo "$a"`, want: "1.50e+00\n",
		},
		{
			// The two numeric letters may stand together and the first
			// written wins. ksh93 answers both orders with its usage block.
			name: "the integer letter first", src: `typeset -iE 3 a=1.5; typeset -p a`,
			want: "typeset -i3 a=1\n",
		},
		{
			name: "the float letter first", src: `typeset -Ei 3 a=1.5; typeset -p a`,
			want: "typeset -E a=1.50e+00\n",
		},
		{
			// A bare letter over a name that already has a precision keeps
			// it; ksh93 resets to the letter's default. Read through a
			// *later* assignment, because the text already stored was
			// rendered at the old precision in both shells and cannot say
			// which attribute the name now carries.
			name: "a bare letter keeps the precision",
			src:  `typeset -E 3 a=1.5; typeset -E a; a=1.23456789; echo "$a"`,
			want: "1.23e+00\n",
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
