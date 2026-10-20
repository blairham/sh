// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `read -d` takes a delimiter of more than one byte whole under a UTF-8
// locale, and its first byte alone under C. See
// interp.Semantics.ReadDelimiterIsTheLocalesCharacter.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc` (#5153).
func TestReadTakesAWideDelimiterWhole(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"two records", `print -n "first£second£" | { read -d £ one; read -d £ two; print $one; print $two }`, "first\nsecond\n"},
		{"and what follows is the next read's", `printf "aéb" | { read -d é x; print $x; read y; print $y }`, "a\nb\n"},
		{"an escaped one does not end the read", `printf "a\\\\£b£c" | { read -d £ x; print -r -- ${x[-1]} }`, "b\n"},
		{"raw", `print -n "x£y" | { read -r -d £ x; print $x }`, "x\n"},
		{"under C, the first byte", `LC_ALL=C; print -n "first£second£" | { read -d £ one; read -d £ two; print $one; print $two }`, "first\n\xa3second\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}
