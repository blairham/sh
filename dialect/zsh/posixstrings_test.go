// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// `setopt posixstrings` makes a `$'…'` string end at its first NUL, so
// `a$'b\0c'd` is the three characters `abd`.
//
// Every row prints each character's code in hexadecimal rather than the
// string itself, because a NUL is invisible in a comparison of text and a row
// that compared the rendering could not tell a byte that is there from one
// that is not. Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0), `-f`,
// 2026-09-26; the option *off* is the control on every row and already
// agreed (#4624).
func TestPosixStringsEndsADollarSingleStringAtItsNul(t *testing.T) {
	const show = "show() { local v=$1; local i; " +
		"for (( i = 1; i <= $#v; i++ )); do c=$v[$i]; printf '%s ' $(( [#16] #c )); done; print }\n"
	for _, tc := range []struct{ name, src, want string }{
		{
			"the string is cut and the word around it is kept",
			"setopt posixstrings\nv=a$'b\\0c'd; show $v\n",
			"16#61 16#62 16#64 \n",
		},
		{
			"and with the option off the NUL is a byte of the string",
			"unsetopt posixstrings\nv=a$'b\\0c'd; show $v\n",
			"16#61 16#62 16#0 16#63 16#64 \n",
		},
		{
			"a string that is nothing but a NUL is nothing",
			"setopt posixstrings\nw=$'\\0'; show $w\n",
			"\n",
		},
		{
			"each string in a word is cut at its own NUL",
			"setopt posixstrings\ny=a$'b\\0c'$'d\\0e'f; show $y\n",
			"16#61 16#62 16#64 16#66 \n",
		},
		{
			"and a string with no NUL in it is untouched",
			"setopt posixstrings\nt=$'no'; show $t\n",
			"16#6E 16#6F \n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), show+tc.src)
			if st != 0 || out != tc.want {
				t.Errorf("out %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// The moment is the **expansion** and not the parse, which is what says the
// option can be read at all where a script sets it after the function that
// uses it was defined. The pair holds the state at the definition fixed at
// the opposite value in each direction.
func TestPosixStringsIsReadWhenTheStringIsExpanded(t *testing.T) {
	const src = "show() { local v=$1; local i; " +
		"for (( i = 1; i <= $#v; i++ )); do c=$v[$i]; printf '%s ' $(( [#16] #c )); done; print }\n" +
		"f() { v=a$'b\\0c'd; show $v }\n" +
		"setopt posixstrings\nf\nunsetopt posixstrings\nf\n"
	out, st := runZsh(t, t.TempDir(), src)
	want := "16#61 16#62 16#64 \n16#61 16#62 16#0 16#63 16#64 \n"
	if st != 0 || out != want {
		t.Errorf("out %q status %d, want %q — one definition, two states", out, st, want)
	}
}

// The emulations turn it on by their own defaults, which is the row that says
// the name is wired to the emulation table as well as to `setopt`.
func TestPosixStringsFollowsTheEmulation(t *testing.T) {
	const src = `emulate sh
[[ -o posixstrings ]] && print sh=on || print sh=off
emulate zsh
[[ -o posixstrings ]] && print zsh=on || print zsh=off`
	out, st := runZsh(t, t.TempDir(), src)
	if want := "sh=on\nzsh=off\n"; st != 0 || out != want {
		t.Errorf("out %q status %d, want %q", out, st, want)
	}
}
