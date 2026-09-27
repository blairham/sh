// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A non-ASCII **character** is written as itself in every listing surface
// here, and a high byte that is not one is spelled out — #4770.
//
// Measured 2026-09-27 on `/bin/ksh`, `Version AJM 93u+ 2012-08-01`, from a
// script file under `env -i PATH=/usr/bin:/bin`; `go version -m` says *not a
// Go executable* for it and `github.com/blairham/sh/cmd/ksh` for ours.
//
// **The locale is the input that decides it**, which is why the axis read the
// other way for as long as it did. One binary, the locale the only thing that
// moved:
//
//	                         LC_ALL=C              LC_ALL=en_US.UTF-8
//	u=é; typeset -p u        u=$'\xc3\xa9'         u=é
//	w=$'\xc3\xa9'            w=$'\xc3\xa9'         w=é
//	alias al=é; alias al     al=$'\xc3\xa9'        al=é
//	m[é]=é; typeset -p m     [$'\xc3\xa9']=$'…'    [é]=é
//	v=$'\xc3'; typeset -p v  v=$'\xc3'             v=$'\xc3'
//
// The recorded value came from the first column — the corpus harness pins
// `LC_ALL=C` — and its stated reason was that this shell spells the byte out
// `in every locale`, which the second column contradicts. bash answers the
// same axis the same way round, and `dialect/bash` already states the choice
// this now follows: the preset carries the reading a person's terminal sees,
// and the corpus goes on recording the `C` cell.
//
// The last row is the control and it is what makes this `Yes` rather than a
// third answer: a high byte that is **not** part of a character is spelled
// out in both locales, which is the distinction #4521 drew for the two
// columns that already answered `Yes`.
//
// **What a `$'...'` spells such a character as is a second question and is
// not here**: a word that needs quoting *and* holds one is `$'a \u[e9] b'`
// in a UTF-8 locale — a code-point form nothing in this engine writes — and
// so is `$'\xc3\xa9\xff'`, where the stray byte is what forces the form.
// Both are #4807. The axis decides whether the byte may stand bare, and those
// rows are about the escape.
func TestAListedNonAsciiCharacterIsWrittenAsItself(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a scalar", `u=é; typeset -p u`, "u=é\n"},
		{"the same value written as bytes", `w=$'\xc3\xa9'; typeset -p w`, "w=é\n"},
		{"an alias body", `alias al=é; alias al`, "al=é\n"},
		{
			"a table's key and its value",
			`typeset -A m; m[é]=é; typeset -p m`,
			"typeset -A m=([é]=é)\n",
		},
		// The control, in three shapes: a byte that cannot be a character on
		// its own is escaped wherever it stands, and a value holding both
		// writes the character and escapes the byte.
		{"a lone lead byte", `v=$'\xc3'; typeset -p v`, "v=$'\\xc3'\n"},
		// `\xc1z` and not `\xc1b`: this shell's `\x` reads as many hex digits
		// as it finds, so `\xc1b` is one three-digit code point and the row
		// would be about the escape rather than about the byte.
		{"one in the middle of a value", `y=$'a\xc1z'; typeset -p y`, "y=$'a\\xc1z'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
