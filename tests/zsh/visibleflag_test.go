// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"runtime"
	"testing"
)

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
	// The last row runs under no locale, so its two high bytes are the C
	// library's to classify: macOS calls them printing and glibc does not,
	// measured on zsh 5.9.2 on both — see interp/clocaleprints_darwin.go.
	hello := "[h\\M-C\\M-)llo]\n"
	if runtime.GOOS == "darwin" {
		hello = "[héllo]\n"
	}
	want := "[a\\tb]\n[a\\nb]\n[a^[b]\n[a^Mb]\n[a$'\\t'b]\n" + hello
	if out != want || st != 0 {
		t.Errorf("(V) = %q (status %d), want %q", out, st, want)
	}
}
