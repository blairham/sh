// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// Which *next word* is an option letter's number, and which is the name to
// read into.
//
// **The test is the first character and not the whole word.** The two agree on
// every word that is plainly one or plainly the other, and they part on a word
// that starts numeric and is not a number — which is where both of `read`'s
// number letters live. Measured 2026-09-28 on zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` *not a Go executable*:
//
//	read -k x v     one character into `x` — never the argument
//	read -k .5 v    the same; `.5` is a name
//	read -k -1 v    the same
//	read -k 007 v   seven characters — the argument
//	read -k 0.5 v   **number expected after -k: 0.5** — taken, then refused
//	read -t 0.5 v   the timeout — taken, and a decimal is fine for that letter
//
// The last two are what a whole-word test gets wrong, and in opposite
// directions: it reads `0.5` as a *name* where one letter refuses it and the
// other accepts it. So a word is taken when it *looks* like a number, and
// whether it is one is the letter's own question afterwards.
func TestWhichNextWordIsAnOptionLettersNumber(t *testing.T) {
	for _, tc := range []struct {
		word string
		want bool
	}{
		// Plainly the argument, under either reading.
		{"2", true},
		{"007", true},
		{"0", true},
		{"12345", true},
		// Plainly a name, under either reading.
		{"x", false},
		{"v", false},
		{"abc", false},
		{"", false},
		// **The rows that part them.** Each begins with a digit and is not an
		// integer, and each is the argument in the reference.
		{"0.5", true},
		{"2.5", true},
		{"1.", true},
		{"1e2", true},
		{"0x2", true},
		// And each of these does *not* begin with a digit, so it is a name
		// however numeric it looks — which is the half a "does it parse"
		// test would get wrong in the other direction.
		{".5", false},
		{"-1", false},
		{"+1", false},
		{" 2", false},
	} {
		t.Run(tc.word, func(t *testing.T) {
			if got := beginsWithADigit(tc.word); got != tc.want {
				t.Errorf("beginsWithADigit(%q) = %v, want %v", tc.word, got, tc.want)
			}
		})
	}
}
