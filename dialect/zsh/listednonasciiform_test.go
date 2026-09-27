// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
)

// The control on ksh93's two listing rules — #4807. This column writes a
// character above ASCII as **itself** inside a `$'...'` a control byte put it
// in, and nothing standing in front of such a character reaches for the form.
//
// Measured 2026-09-27 under `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8` on zsh 5.9.2 (aarch64-apple-darwin25.4.0), run `-f`.
//
// Without these the ksh93 rows would be a rule this engine applied everywhere
// and nothing would say so.
func TestANonAsciiCharacterIsWrittenAsItselfHere(t *testing.T) {
	sem := zsh.Semantics()
	if got := sem.ListedNonAsciiIsSpelledAsACodePoint; got != interp.No {
		t.Errorf("ListedNonAsciiIsSpelledAsACodePoint = %v, want No", got)
	}
	if got := sem.ListedNonAsciiTakesTheDollarFormAfterANonName; got != interp.No {
		t.Errorf("ListedNonAsciiTakesTheDollarFormAfterANonName = %v, want No", got)
	}
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a hyphen in front of it", `v="a-é"; typeset -p v`, "typeset v=a-é\n"},
		{"inside a form a tab forced", `v=$'a\téb'; typeset -p v`, "typeset v=$'a\\téb'\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("%s = %q (status %d), want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
