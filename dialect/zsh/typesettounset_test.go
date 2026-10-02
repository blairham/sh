// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`typesettounset` leaves a valueless declaration unset, and the listings
// say so** (#5157). Measured 2026-10-01 on zsh 5.9.2 under `-f -c`, byte for
// byte. The option is Semantics.DeclaredNameWithoutValueIsEmpty read
// backwards, and the emulations' own readings of that axis are its default in
// each: on under sh and ksh, off under csh and zsh.
func TestTypesetToUnset(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"the option", `setopt typesettounset; typeset X; integer i; typeset -a a; typeset -A m
echo "[${X+set}] [${i+set}] [${a+set}] [${m+set}]"; typeset -p X i a m`,
			"[] [] [] []\ntypeset X\ntypeset -i i\ntypeset -a a\ntypeset -A m\n",
		},
		{"off under sh", `emulate sh; unsetopt typesettounset; typeset X; echo "[${X+set}]"`, "[set]\n"},
		{"on under csh", `emulate csh; setopt typesettounset; typeset X; echo "[${X+set}]"`, "[]\n"},
		{
			"the listings, under sh",
			`emulate sh; f() { local -t s; local -a a; local -i i; local x
typeset -m s a i x; echo ---; typeset +m s a i x; echo ---; typeset -p s a i x; echo ---
typeset + | /usr/bin/grep -E " (s|a|i|x)$"; echo ---; typeset | /usr/bin/grep -E "(local|^)[^=]* (s|a|i|x)(=|$)"; }; f`,
			"---\n---\ntypeset -t s\ntypeset -a a\ntypeset -i i\ntypeset x\n---\n" +
				"array local a\ninteger local i\nlocal tagged s\nlocal x\n---\n" +
				"array local a\ninteger local i\nlocal tagged s\nlocal x\n",
		},
		{
			"control: zsh's own mode", `typeset X; typeset -a a; echo "[${X+set}] [${a+set}]"; typeset -p X a`,
			"[set] [set]\ntypeset X=''\ntypeset -a a=(  )\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, out, c.want)
			}
		})
	}
}
