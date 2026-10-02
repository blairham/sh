// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnOperandsKeyIsReadAsWritten pins that a key reaching a builtin as text
// keeps its quote characters, as the same subscript written on a line of its
// own does, while any expansion in it is still performed (#5152). Measured
// 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAnOperandsKeyIsReadAsWritten(t *testing.T) {
	const setup = "typeset -A h; x=V\nkeys() { for k in \"${(@ok)h}\"; do print -rn -- \"<$k>\"; done; print }\n"
	for _, tc := range []struct{ src, want string }{
		{`typeset -g "h[a\"b]"=1; keys`, "<a\"b>\n"},
		{`typeset -g 'h[c\"d]'=2; keys`, "<c\\\"d>\n"},
		{`typeset -g 'h[g\\h]'=3; keys`, "<g\\h>\n"},
		{`typeset -g 'h[$x]'=4; keys`, "<V>\n"},
		{`typeset -g 'h["$x"]'=5 'h[a"$x]'=6; keys`, "<\"V\"><a\"V>\n"},
		{`typeset -g 'h[$(echo "a b")]'=7 "h[j'k]"=8; keys`, "<a b><j'k>\n"},
		{`builtin typeset -g "h[e\"f]"=9; keys`, "<e\"f>\n"},
		{`y='h[m"n]=1'; typeset -g $y; keys`, "<m\"n>\n"},
		{`read 'h[x"y]' <<< R; keys`, "<x\"y>\n"},
		{`printf -v 'h[z"w]' %s P; keys`, "<z\"w>\n"},
		{`h=('a"b' 1); [[ -v 'h[a"b]' ]] && print set`, "set\n"},
		{`h=('a"b' 1 'c\"d' 2); unset 'h[a"b]'; keys`, "<c\\\"d>\n"},
		// The control: a key with nothing for the two readings to part over.
		{`typeset -g 'h[plain]'=1; keys`, "<plain>\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// TestAnExpansionInASubscriptSpendsAnEscapedQuote pins that an expansion
// written inside another's subscript reads its own key with a `\"` spent,
// quoted or not, and its search pattern with it spent only inside double
// quotes (#5152). Measured 2026-10-02 on zsh 5.9.2.
func TestAnExpansionInASubscriptSpendsAnEscapedQuote(t *testing.T) {
	const setup = "typeset -A h; h=('k\"m' A 'k\\\"m' B A fromA B fromB 'k\\m' P P fromP)\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- $h[k\"m] "$h[k\"m]"`, "B A\n"},
		{`print -r -- $h[$h[k\"m]] "$h[$h[k\"m]]" "${h[$h[k\"m]]}"`, "fromA fromA fromA\n"},
		{`print -r -- "$h[(i)k\"m]" "$h[$h[(i)k\"m]]" $h[$h[(i)k\"m]]`, "k\\\"m A B\n"},
		{`print -r -- ${h[$h[(e)k\"m]]} $h[$h[k\\m]] $h[(k)k\"m] $h[$h[(k)k\"m]]`, "fromA fromP B fromA\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
