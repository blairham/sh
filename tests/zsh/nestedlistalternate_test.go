// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A `+` over a nested expansion whose inner came to a **list** is its word or
// nothing, and never the list (#5866). With nothing written behind the `+`,
// the list came back as though no operator had been written — so
// powerlevel10k's `Dev${$((_p9k__d+=6))+}` drew `Dev-11` where zsh draws
// `Dev`.
//
// What decides it is whether the inner is a list, not which kind of
// expansion it is and not where it is written: an arithmetic or command
// substitution is a list unquoted and a string quoted, an array parameter is
// a list and a scalar is not, and a replacement word is read unquoted. The
// pairs below hold one of those fixed and move the other — `${${a}+}` against
// `${${y}+}`, `${$((7))+}` against `"${$((7))+}"` — and the `-` rows are the
// other side of the same test, where the list is the answer.
//
// Every row measured 2026-10-04 on zsh 5.9.2 with `zsh -f -c`.
func TestAnEmptyAlternateOverANestedListIsNothing(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`printf '<%s>' x ${$((7))+} y`, "<x><y>"},
		{`printf '<%s>' x ${$(echo 7)+} y`, "<x><y>"},
		{`printf '<%s>' x ${$((7)):+} y`, "<x><y>"},
		{`printf '<%s>' x ${$(echo a b)+} y`, "<x><y>"},
		{`a=(p q); printf '<%s>' x ${${a}+} y`, "<x><y>"},
		{`a=(p q); printf '<%s>' x ${${a}:+} y`, "<x><y>"},
		{`a=(p q); printf '<%s>' x ${${a[@]}+} y`, "<x><y>"},
		{`a=(p q); printf '<%s>' x "${${a[@]}+}" y`, "<x><><y>"},
		{`v=${$((7))+}; printf '<%s>' "$v"`, "<>"},
		{`v=a${$(echo 7)+}b; printf '<%s>' "$v"`, "<ab>"},
		{`x=aXb; printf '<%s>' "${x/X/${$((7))+}}"`, "<ab>"},
		{`x=aXb; printf '<%s>' "${x//X/${$((7))+}}"`, "<ab>"},
		{`x=aXb; printf '<%s>' "${x/X/z${$((7))+}}"`, "<azb>"},
		{`x=aXb; printf '<%s>' ${x/X/${$((7))+}}`, "<ab>"},
		{`x=aXb; printf '<%s>' "${x/X/${$((7)):+k}}"`, "<akb>"},
		{`x=aXb; printf '<%s>' "${x/X/${$((7))+k}}"`, "<akb>"},
		{`x=aXb; printf '<%s>' "${x/X/${$((7))-k}}"`, "<a7b>"},
		{`x=aXb; printf '<%s>' "${x/X/${$((7))-}}"`, "<a7b>"},
		{`printf '<%s>' x ${$((7))-} y`, "<x><7><y>"},
		{`printf '<%s>' x ${$(echo a b)-} y`, "<x><a><b><y>"},
		{`a=(p q); printf '<%s>' x ${${a}-} y`, "<x><p><q><y>"},
		{`a=(p q); printf '<%s>' x ${${a}:-z} y`, "<x><p><q><y>"},
		{`y=1; printf '<%s>' x ${${y}+} y`, "<x><y>"},
		{`printf '<%s>' x "${$((7))+}" y`, "<x><><y>"},
		// The inner runs once: the test that chose the side read it through
		// the hold the value is read through.
		{`printf '<%s>' ${$(print -n x >>c; echo 7)+} "$(<c)"`, "<x>"},
		{`d=-1; l=/a/Developer/b; printf '<%s>' "${l/Developer/${${${d:#-*}:+Developer}:-Dev${$((d+=6))+}}}" $d`, "</a/Dev/b><5>"},
	} {
		t.Run(tc.src, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
			}
		})
	}
}
