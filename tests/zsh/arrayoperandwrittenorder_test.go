// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A declaration's array-literal operand is expanded where it is written,
// between the words either side of it, although zsh places it only once the
// utility has run. Measured 2026-10-30 against zsh 5.9.2 from script files
// under `env -i PATH=/usr/bin:/bin`, byte for byte apart from the file's path;
// GNU bash 5.3.20, bash 3.2.57 and ksh93u+ expand in the same order. The
// literals were expanded after every other word, so `s` came out ahead of `A`
// and a literal that failed did so after `t` had run (#7075).
func TestADeclarationExpandsItsLiteralOperandWhereItIsWritten(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{
			"a literal before a scalar, which is then refused",
			"typeset -a a=($(echo A >&2)) y=$(echo s >&2)\n",
			"A\ns\nf.sh:typeset:1: y: inconsistent type for assignment\n",
		},
		{
			"a literal between two scalars",
			"typeset y=$(echo s >&2) a=($(echo A >&2)) z=$(echo t >&2)\n",
			"s\nA\nt\n",
		},
		{
			"a literal that fails stops the words behind it",
			"typeset y=$(echo s >&2) a=(1 $((1/0)) $(echo A >&2)) z=$(echo t >&2)\n",
			"s\nf.sh:1: division by zero\n",
		},
		{
			"a literal is still not placed before the next word expands",
			"a=(0); typeset -a a=(1) b=(\"${a[@]}\"); print -r -- \"$b\"\n",
			"0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := runZshScriptFile(t, c.src, "f.sh"); got != c.want {
				t.Errorf("%s\ngot  %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
