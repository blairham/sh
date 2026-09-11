// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${(V)x}` through the shell that has the flag, against zsh 5.9.2.
//
// The ordering row is the one worth having here rather than beside the
// function: `(V)` runs **after** `(q)`, measured — `${(Vq)}` on a tab is
// `a$'\t'b`, so the quoting saw the control character and this did not.
// Reversed it would be `a\\tb`, which is a plausible answer and the wrong
// one.
func TestTheVisibleFlagMakesControlCharactersVisible(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `s=$'a\tb'
print -r -- "[${(V)s}]"
n=$'a\nb'
print -r -- "[${(V)n}]"
e=$'a\eb'
print -r -- "[${(V)e}]"
r=$'a\rb'
print -r -- "[${(V)r}]"
print -r -- "[${(Vq)s}]"
u=héllo
print -r -- "[${(V)u}]"`)
	want := "[a\\tb]\n[a\\nb]\n[a^[b]\n[a^Mb]\n[a$'\\t'b]\n[héllo]\n"
	if out != want || st != 0 {
		t.Errorf("(V) = %q (status %d), want %q", out, st, want)
	}
}
