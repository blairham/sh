// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A redirection's target is brace-expanded here, and what the braces make is
// several *targets* rather than an ambiguity.
//
// Measured 2026-09-25 against zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`,
// aarch64-apple-darwin25), each row in an empty directory of its own — run
// two shells in one directory and the first one's files are still there when
// the second runs, which hides exactly this bug:
//
//	: > {a,b,c}                        three files, `a`, `b` and `c`
//	echo hi > {a,b}                    both files, each holding `hi`
//	: > {a,b}c                         `ac` and `bc`
//	: > {1..3}                         `1`, `2` and `3`
//	: > "{a,b}"                        one file called `{a,b}`
//	e="{a,b}"; : > $e                  one file called `{a,b}`
//
// The last two are the half that says this is brace *syntax* and not a rule
// about the characters: quoting hides the braces from the scanner, and text
// that arrived from a parameter was never source to begin with. They are the
// same two rows brace expansion answers in an argument, which is the point —
// the target was simply never put through it, so `: > d/{a,b,c}` made one
// file whose name held the braces (#4455).
//
// bash reaches the same expansion and refuses what it makes, because it reads
// a target as an ordinary word: see dialect/bash and dialect/ksh for the two
// other answers in the panel.
func TestARedirectionTargetIsBraceExpandedAndNamesEveryFile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the line this was made for",
			`: > {a,b,c}; printf '[%s]' *`,
			"[a][b][c]",
		},
		{
			// MULTI_IOS: the one redirection opens all three, so the write
			// reaches every one of them rather than only the last.
			"and every one of them is written",
			`echo hi > {a,b}; read x < a; read y < b; printf '[%s]' "$x" "$y"`,
			"[hi][hi]",
		},
		{"text behind the group", `: > {a,b}c; printf '[%s]' *`, "[ac][bc]"},
		{"a range", `: > {1..3}; printf '[%s]' *`, "[1][2][3]"},
		{
			// The endpoints are expanded before the range is read here,
			// which is this dialect's answer in an argument too.
			"a range whose end is an expansion",
			`f() { echo 3; }; : > {1..$(f)}; printf '[%s]' *`,
			"[1][2][3]",
		},
		{"a group with no comma is not a group", `: > {a}; printf '[%s]' *`, "[{a}]"},
		{"quoted braces are not syntax", `: > "{a,b}"; printf '[%s]' *`, "[{a,b}]"},
		{
			"and braces that arrived from a parameter are not either",
			`e="{a,b}"; : > $e; printf '[%s]' *`,
			"[{a,b}]",
		},
		{
			// The switch reaches the target: with brace expansion off the
			// word is the one name it is written as.
			"the option turns it off",
			`unsetopt braceexpand; : > {a,b}; printf '[%s]' *`,
			"[{a,b}]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s gave %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
