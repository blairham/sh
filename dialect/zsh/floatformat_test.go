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

// The float letters decide how a number is **written**, not what it is.
//
// The discriminating read is the one that does not go through the letter:
// arithmetic. A test that only ever reads a float name back through the same
// rendering cannot tell a rounded store from a rounded print, because it is
// comparing the rendering with itself — so every row here either reads the
// number through `$(( ))` or writes it again at a *different* precision.
//
// Measured 2026-09-25 against zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`
// with no startup files. See interp/floatformat.go and #4475, where the
// stored value was the rendering and `float -E3 f; float -F f` wrote `3.140`
// for a name holding 3.14159265358979.
func TestAFloatLetterDecidesHowItIsWrittenNotWhatItIs(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{
			// The reduction from the issue, and the one row that needs no
			// arithmetic: the second letter writes a digit the first letter
			// was not printing, which it can only do if that digit is still
			// there.
			name: "a later letter writes a digit the earlier one hid",
			src:  `float f=3.14159; float -E3 f; print $f; float -F f; print $f; typeset -p f`,
			want: "3.14e+00\n3.142\ntypeset -F f=3.142\n",
		},
		{
			name: "arithmetic reads the number and not the rendering",
			src:  `typeset -E3 f=3.14159265358979; print $f; print $(( f ))`,
			want: "3.14e+00\n3.14159265358979\n",
		},
		{
			// The sharpest of them: the rendering says `3.1`, the doubling
			// is of every digit, and the new rendering says `6.3` rather
			// than the `6.2` a doubled `3.1` would give. One line that
			// fails whichever half is wrong.
			name: "an arithmetic assignment reads and writes the number",
			src:  `typeset -F1 f=3.14159265358979; (( f = f * 2 )); print $f; print $(( f ))`,
			want: "6.3\n6.28318530717958\n",
		},
		{
			name: "an append adds to the number",
			src:  `typeset -F1 f=3.14159265358979; f+=1; print $f; print $(( f ))`,
			want: "4.1\n4.1415926535897896\n",
		},
		{
			name: "and so does an arithmetic append",
			src:  `typeset -F1 f=3.14159265358979; (( f += 1 )); print $(( f ))`,
			want: "4.1415926535897896\n",
		},
		{
			// Three renderings of one number, the third of which cannot be
			// written from the second's characters.
			name: "a precision changed twice still writes the number",
			src:  `typeset -F3 f=3.14159265358979; typeset -p f; typeset -E5 f; typeset -p f; typeset -F f; typeset -p f`,
			want: "typeset -F f=3.142\ntypeset -E f=3.1416e+00\ntypeset -F f=3.14159\n",
		},
		{
			// The attribute arriving at a name that already holds a value
			// reads the value, and a later letter then writes it out in
			// full — the same rule from the other end.
			name: "an attribute added to a standing value",
			src:  `float f=3.14159265358979; typeset -E3 f; print $f; typeset -F 10 f; print $f`,
			want: "3.14e+00\n3.1415926536\n",
		},
		{
			// An ordinary assignment is *not* a re-read: what is assigned is
			// what the name holds, even where the characters happen to be
			// the ones the old rendering wrote.
			name: "an assignment stores what it was given",
			src:  `typeset -F1 f=3.14159265358979; f=3.1; print $(( f ))`,
			want: "3.1000000000000001\n",
		},
		{
			// And the plus form is where the number really is destroyed: it
			// leaves a scalar holding characters, and the letter coming back
			// can only read those.
			name: "the plus form leaves characters behind",
			src:  `typeset -F1 f=3.14159265358979; typeset +F f; typeset -F1 f; print $(( f ))`,
			want: "3.1000000000000001\n",
		},
		{
			// A local is a fresh binding, and the caller's number comes back
			// with the caller's letter — neither taken by the call nor
			// rounded by it.
			name: "a call neither takes the number nor rounds it",
			src: `typeset -E3 f=3.14159265358979; g(){ typeset -F1 f=2.7182818; print $(( f )); }; g; ` +
				`print $f; print $(( f ))`,
			want: "2.7182818000000002\n3.14e+00\n3.14159265358979\n",
		},
		{
			// The rendering is still what is *stored*, which is the half
			// that must not move: the eight characters of `3.14e+00` are
			// what a length counts and what a listing writes.
			name: "the rendering is what is stored",
			src:  `typeset -E3 f=3.14159265358979; print ${#f}; typeset -p f`,
			want: "8\ntypeset -E f=3.14e+00\n",
		},
		{
			// A subscripted store replaces the whole of it and folds like
			// any other assignment, so the number it leaves behind is the
			// one it was given rather than the one it printed.
			name: "a subscripted store replaces the whole of it",
			src:  `typeset -F1 f=3.14159265358979; f[2]=9.87654321; print $f; print $(( f ))`,
			want: "9.9\n9.8765432099999995\n",
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
