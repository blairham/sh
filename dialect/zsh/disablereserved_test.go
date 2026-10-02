// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestDisableRSwitchesADeclarationWord: `disable -r typeset` makes `typeset`
// the builtin, so its operands are ordinary words; `enable -r` puts the
// reserved word back, which runs even where the builtin is disabled. The
// tables follow: `whence -w` says builtin, `$reswords` loses the word and
// `$dis_reswords` holds it. A reserved word this shell keeps no table for is
// refused by name, and a name that is none is no such element. Measured
// 2026-10-02 on zsh 5.9.2, B02typeset's `reserved word and builtin
// interfaces` (#5142).
func TestDisableRSwitchesADeclarationWord(t *testing.T) {
	const fn = "fn='fn() { typeset foo=`echo one word=two`; print -r -- \"[$foo] [$word]\" }'\n"
	for _, tc := range []struct{ src, want string }{
		{
			fn + `eval $fn; fn; disable -r typeset; eval $fn; fn; enable -r typeset; disable typeset; eval $fn; fn`,
			"[one word=two] []\n[one] [two]\n[one word=two] []\n",
		},
		{
			`zmodload zsh/parameter; disable -r typeset local; disable -r; print ${#reswords} $dis_reswords; whence -w typeset`,
			"local\ntypeset\n29 typeset local\ntypeset: builtin\n",
		},
		{`disable -r nosuch; print $?`, "zsh:disable:1: no such hash table element: nosuch\n1\n"},
		{`disable -r if; print $?`, "zsh:disable:1: -r if is not implemented yet\n2\n"},
	} {
		out, _ := runZsh(t, t.TempDir(), tc.src)
		if out != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
		}
	}
}
