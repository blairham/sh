// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A byte a value carries that is not a character on its own is written as an
// escape, where this shell wrote it raw.
//
// The control is the byte either side of the boundary: a control character was
// already escaped here, and a *valid* multi-byte character is written as
// itself by the reference — so what was missing is the high half and not the
// escaping (#4521). Measured 2026-09-26 on zsh 5.9.2 under `-f` from a script
// file; `go version -m` says *not a Go executable* for it and
// `github.com/blairham/sh/cmd/zsh` for ours.
func TestAListedByteThatIsNotACharacterIsEscaped(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a lone lead byte", `v=$'\xc3'; typeset -p v`, "typeset v=$'\\M-C'\n"},
		{"one in the middle of a value", `w=$'a\xc1b'; typeset -p w`, "typeset w=$'a\\M-Ab'\n"},
		{"the high end of the range", `x=$'\xff'; typeset -p x`, "typeset x=$'\\M-\\C-?'\n"},
		// The prefix is a prefix on the ordinary spelling rather than a
		// vocabulary of its own, which these four say between them.
		{"a high byte whose low half is a control", `v=$'\x81'; typeset -p v`, "typeset v=$'\\M-\\C-A'\n"},
		{"a high byte whose low half is a tab", `v=$'\x89'; typeset -p v`, "typeset v=$'\\M-\\t'\n"},
		{"a high byte whose low half is a space", `v=$'\xa0'; typeset -p v`, "typeset v=$'\\M- '\n"},
		{"a high byte whose low half is printable", `v=$'\xe9'; typeset -p v`, "typeset v=$'\\M-i'\n"},
		// And the two controls, which are what say this is the high half and
		// not the escaping.
		{"a control byte was already escaped", `y=$'\x01'; typeset -p y`, "typeset y=$'\\C-A'\n"},
		{"delete too", `q=$'\x7f'; typeset -p q`, "typeset q=$'\\C-?'\n"},
		{"and a character is still written as itself", `u=é; typeset -p u`, "typeset u=é\n"},
		{
			"a value holding both writes the character and escapes the byte",
			`z=$'\xc3\xa9\xff'; typeset -p z`,
			"typeset z=$'é\\M-\\C-?'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}

// A bare `set` spells an association's key the way `typeset -p` on the same
// table does.
//
// They were two spellings of one value in one shell: the bare listing reached
// the *clustered* key function, which is the other dialect's shape, so a key
// holding a quote came back double-quoted from the bare listing and in
// single-quoted runs from the named one (#4732). The value is right on every
// row and in both listings, which is what narrows it to the key. Measured
// 2026-09-26 on zsh 5.9.2 under `-f`.
func TestTheBareListingSpellsATableKeyAsTheDeclarationDoes(t *testing.T) {
	rows := func(src string) string { return src }
	for _, tc := range []struct{ name, src, want string }{
		{
			"a key holding a quote",
			rows(`typeset -A m=(["k'1"]="v'2")
			      typeset -p m
			      set | while IFS= read -r l; do case $l in (m=*) print -r -- "$l";; esac; done`),
			"typeset -A m=( ['k'\\''1']='v'\\''2' )\nm=( ['k'\\''1']='v'\\''2' )\n",
		},
		{
			// A key holding nothing but a space comes back the same way,
			// which is what says the quote is not what decides it.
			"a key holding a space",
			rows(`typeset -A n=(["a b"]=x)
			      typeset -p n
			      set | while IFS= read -r l; do case $l in (n=*) print -r -- "$l";; esac; done`),
			"typeset -A n=( ['a b']=x )\nn=( ['a b']=x )\n",
		},
		{
			// The operand-less listings take the same road, which is the
			// half a fix aimed only at `set` would have left behind.
			"and the operand-less listings too",
			rows(`typeset -A p=(["a b"]=x)
			      typeset -A | while IFS= read -r l; do case $l in (p=*) print -r -- "$l";; esac; done
			      typeset -Ar ro=(["p q"]=1)
			      typeset -r | while IFS= read -r l; do case $l in (ro=*) print -r -- "$l";; esac; done`),
			"p=( ['a b']=x )\nro=( ['p q']=1 )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
