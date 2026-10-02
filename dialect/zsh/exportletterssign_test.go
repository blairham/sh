// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestTheExportLetterTakesItsOwnSign pins three rows about the export
// attribute on a declaration line, measured 2026-10-02 on zsh 5.9.2 under
// `-f`: a `+x` word is not turned around by a minus word after it, a name the
// function already holds keeps its export through a later declaration of it,
// and a fresh local tie holds no element.
func TestTheExportLetterTakesItsOwnSign(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"export e=1; typeset +x -i e; typeset -p e", "typeset -i e=1\n"},
		{"export e=1; typeset -i +x e; typeset -p e", "typeset -i e=1\n"},
		{"export G=1; typeset +x -T G g; typeset -p G", "typeset -T G g=( 1 )\n"},
		// The control: the letter under a minus still exports.
		{"e=1; typeset -x -i e; typeset -p e", "export -i e=1\n"},
		{"f(){ typeset FOO=a:b; export FOO; typeset +x -T FOO foo; typeset -p FOO }; f", "typeset -T FOO foo=( a b )\n"},
		{
			"f(){ typeset FOO=a:b; export FOO; typeset -T FOO foo; typeset -p FOO; print ${(t)FOO} }; f",
			"local -xT FOO foo=( a b )\nscalar-local-tied-export\n",
		},
		{"f(){ local FOO=a:b; export FOO; typeset FOO; typeset -p FOO }; f", "FOO=a:b\nlocal -x FOO=a:b\n"},
		{"f(){ local FOO=a:b; export FOO; local FOO; typeset -p FOO }; f", "FOO=a:b\nlocal -x FOO=a:b\n"},
		{"f(){ typeset -T S s; print ${#s}; typeset -p s }; f", "0\ntypeset -aT S s=(  )\n"},
		// And a fresh local over an exported global still answers the axis.
		{"export G=1; f(){ local G; typeset -p G }; f", "typeset G=''\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
