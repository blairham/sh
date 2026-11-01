// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A number after `-i`, `-F` or `-E` with no name behind it is a listing, and
// a base the shell will not spell is refused only where a name would carry
// it. And `local` with a minus letter and no name is `typeset`'s filtered
// listing, not the whole table. Measured 2026-10-16 on zsh 5.9.2, `-f`, under
// `env -i PATH=/usr/bin:/bin` (#6810).
func TestADeclarationNumberWithNoNameIsAListing(t *testing.T) {
	t.Parallel()
	const table = "pick() { print -r -- ${(M)${(f)1}:#$~2}; }\n" +
		"typeset -F fl=1; typeset -E ex=2; integer n=3; typeset -a ar=(q); typeset -x xp=4\n"
	for _, tc := range []struct{ src, want string }{
		// The integer listing, which names n among the shell's own.
		{`o=$(typeset -i 0); echo $?; pick "$o" 'n=*'; pick "$o" 'fl=*'`, "0\nn=3\n\n"},
		{`pick "$(typeset -i99)" 'n=*'`, "n=3\n"},
		{`pick "$(typeset +i 0)" 'n*'`, "n\n"},
		{`f(){ o=$(local -i 0); echo in $?; pick "$o" 'n=*'; }; f`, "in 0\nn=3\n"},
		// A refused base with a name is refused once per name, and neither
		// name is declared.
		{
			`typeset -i0 x y 2>&1; echo $?; echo ${+x} ${+y}`,
			"zsh:typeset:3: invalid base (must be 2 to 36 inclusive): 0\n" +
				"zsh:typeset:3: invalid base (must be 2 to 36 inclusive): 0\n1\n0 0\n",
		},
		// A later base takes the refused one's place before it is asked.
		{`typeset -i0 -i16 x; echo $?; typeset -p x`, "0\ntypeset -i16 x=0\n"},
		// local's filtered listings.
		{`f(){ local -F 0; echo in $?; }; f`, "fl=1.0000000000\nin 0\n"},
		{`f(){ local -E; }; f`, "ex=2.000000000e+00\n"},
		{`f(){ o=$(local -a); pick "$o" 'ar=*'; pick "$o" 'n=*'; }; f`, "ar=( q )\n\n"},
		{`f(){ o=$(local -x); pick "$o" 'xp=*'; pick "$o" 'n=*'; }; f`, "xp=4\n\n"},
		// And the two float letters select apart under `typeset` too.
		{
			`typeset -F; typeset -E 3; typeset +E`,
			"fl=1.0000000000\nex=2.000000000e+00\nex\n",
		},
		// `local -E` declares, as `typeset -E` does.
		{`f(){ local -E v=1.5; typeset -p v; }; f`, "typeset -E v=1.500000000e+00\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), table+tc.src)
		if out != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, out, tc.want)
		}
	}
}
