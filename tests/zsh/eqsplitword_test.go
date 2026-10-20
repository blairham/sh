// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnEqualsSplitOverAnOperatorWordSplitsTheWordAsItExpands pins that a
// `${=…}` or `${==…}` whose word substituted splits — or does not — the word
// as it expands, so the word's own quoting protects, and that an `(A)`
// assignment's yield is not split again (#5151, a chunk of
// D04parameter.ztst). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAnEqualsSplitOverAnOperatorWordSplitsTheWordAsItExpands(t *testing.T) {
	const setup = "set A 'b c'; v='x y'\nl() { print -rn -- \"$#:\"; for w; print -rn -- \"<$w>\"; print }\n"
	for _, tc := range []struct{ src, want string }{
		{`l ${=1+"$@"}`, "2:<A><b c>\n"},
		{`l ${=1+"$v"$v}`, "2:<x yx><y>\n"},
		{`l ${=nosuch-"p q" r s}`, "3:<p q><r><s>\n"},
		{`l "${=1+a b}" "${=1+$v}"`, "3:<a><b><x y>\n"},
		{`l "${=1+"$@"}"`, "2:<A><b c>\n"},
		{`unset foo; l ${(A)=foo=a "k p" b}; l $foo`, "3:<a><k p><b>\n3:<a><k p><b>\n"},
		{
			`f() { emulate -L sh; local p='1 2' q='3 4'; l ${==1:-$p $q}; l ${==1:-"$p" $q}; l ${1:-"$p" $q}; }; f`,
			"1:<1 2 3 4>\n1:<1 2 3 4>\n3:<1 2><3><4>\n",
		},
		// The control: without the flag the word's blank does not split here.
		{`p='1 2'; q='3 4'; l ${nosuch:-$p $q} ${=nosuch:-$p $q}`, "5:<1 2 3 4><1><2><3><4>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
