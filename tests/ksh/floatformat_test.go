// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// `typeset -E n` — this shell's reading, which is `%.*g` with **n**
// significant digits.
//
// Measured 2026-09-15 against ksh93u+ 2012-08-01, `env -i` with a scratch
// HOME and no startup files. The same letter is `%.*e` with n−1 places in
// zsh, and neither rendering is a special case of the other — see
// interp/floatformat.go and dialect/zsh/floatformat_test.go.
//
// It is the **first** numeric letter this shell has been given, which is why
// three axes arrive with it rather than one: the letter itself, the rule that
// says where its number may be written, and what a bare one does. #1461 and
// #2559 both deferred the middle one.
func TestTheExponentLetterIsSignificantDigits(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"three significant digits", `typeset -E 3 a=3.14159; echo "$a"`, "3.14\n"},
		{"a small number", `typeset -E 3 b=0.000123456; echo "$b"`, "0.000123\n"},
		// No exponent where none is needed, which is the discriminating row
		// against zsh's `%e`: that shell writes `1.00e+02` here.
		{"a number that needs no exponent", `typeset -E 3 f=100; echo "$f"`, "100\n"},
		{"one digit", `typeset -E 1 g=3.14159; echo "$g"`, "3\n"},
		{"the default is ten", `typeset -E c=1.5; echo "$c"`, "1.5\n"},
		{"a word that is no number", `typeset -E 3 h=abc; echo "$h"`, "0\n"},
		{"attached to the letter", `typeset -E3 a=3.14159; echo "$a"`, "3.14\n"},
		{"two names on one line", `typeset -E 3 a=3.14159 b=2.5; echo "$a $b"`, "3.14 2.5\n"},
		// A word that is not a run of digits was never the letter's
		// argument, so it is a name of its own — the rule the number-taking
		// letters already had.
		{"a word that is not a number is a name", `typeset -E abc a=1.5; echo "[$a][$abc]"`, "[1.5][]\n"},
		{"an append", `typeset -E 3 a=1.5; a+=1; echo "$a"`, "2.5\n"},
		// The listing writes the number as a **word of its own**, where zsh
		// writes none at all, and the letter sits where the integer base
		// does.
		{"the listing writes the width", `typeset -E 3 a=1.5; typeset -p a`, "typeset -E 3 a=1.5\n"},
		{"after export and readonly", `typeset -xE 3 a=1.5; typeset -p a`, "typeset -x -E 3 a=1.5\n"},
		{"and after readonly", `typeset -rE 3 a=1.5; typeset -p a`, "typeset -r -E 3 a=1.5\n"},
		{"no number, no word", `typeset -E a=1.5; typeset -p a`, "typeset -E a=1.5\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q at %d, want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}

// **A detached number is this letter's argument only where the letter ends
// its option word.** That is the rule #1461 deferred and #2559 named as the
// blocker for every numeric letter under this shell's name, and it is
// Semantics.DeclareNumberDetachedOnlyAtTheWordEnd.
func TestADetachedNumberNeedsTheLetterAtTheWordEnd(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "the letter ends the word", src: `typeset -E 3 a=3.14159; echo "$a"`,
			want: "3.14\n",
		},
		{
			// The same two letters the other way round, and the number is
			// read: it is the position and not the letter that decides.
			name: "another letter in front of it", src: `typeset -xE 3 a=3.14159; echo "$a"`,
			want: "3.14\n",
		},
		{
			// And with a letter behind it the `3` is an operand again, which
			// this shell refuses as a name — fatally, `typeset` being one of
			// its special builtins. zsh reads the number here and discards
			// the `l`.
			name: "a letter behind it", src: `typeset -El 3 a=1.5; echo after`,
			want: "ksh: typeset: 3: invalid variable name\n", status: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if out != tc.want || st != tc.status {
				t.Errorf("%s = %q at %d, want %q at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}

// The other two disagreements the letter brought with it.
func TestTheNumericLettersCompanyAndABareOne(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name, src, want string
		status          int
	}{
		{
			// A bare letter resets the precision to the letter's default,
			// where zsh keeps what the name had. Read through a *later*
			// assignment: the text already stored was rendered at the old
			// precision in both shells and cannot say which attribute the
			// name now carries.
			name: "a bare letter resets the precision",
			src:  `typeset -E 3 a=1.5; typeset -E a; a=1.23456789; echo "$a"`,
			want: "1.23456789\n",
		},
		{
			// The integer letter and a float letter cannot both stand, in
			// either order, and the refusal is the builtin's usage block —
			// which ends the script here. zsh takes the pair and lets the
			// first written win.
			name: "the integer letter first", src: `typeset -iE 3 a=1.5; echo after`,
			want: "Usage: typeset", status: 2,
		},
		{
			name: "the float letter first", src: `typeset -Ei 3 a=1.5; echo after`,
			want: "Usage: typeset", status: 2,
		},
		{
			// `integer` is `typeset -li` under another word, so the pair
			// arrives there without the letter being written twice.
			name: "and under the integer word", src: `integer -E 3 a=1.5; echo after`,
			want: "Usage: typeset", status: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, tc.src)
			if st != tc.status || len(out) < len(tc.want) || out[:len(tc.want)] != tc.want {
				t.Errorf("%s = %q at %d, want %q… at %d", tc.src, out, st, tc.want, tc.status)
			}
		})
	}
}
