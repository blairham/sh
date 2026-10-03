// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAnUnsetIndirectionNamesWhatItResolvedTo pins that the NO_UNSET and `?`
// refusals of a `(P)` expansion name the parameter the indirection resolved
// to, written as the text it came from, where an unset base names itself.
// Measured 2026-10-03 on zsh 5.9.2 under `-f`.
func TestAnUnsetIndirectionNamesWhatItResolvedTo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`(setopt nounset; s=a; print ${(P)s}); echo st=$?`, "zsh:1: a: parameter not set\nst=1\n"},
		{`(setopt nounset; n=(a b); print ${(P)n}); echo st=$?`, "zsh:1: a b: parameter not set\nst=1\n"},
		{`(setopt nounset; n=(); print ${(P)n}); echo st=$?`, "zsh:1: : parameter not set\nst=1\n"},
		{`(n=(a b); print ${(P)n:?boom}); echo st=$?`, "zsh:1: a b: boom\nst=1\n"},
		{`(setopt nounset; print ${(P)nope}); echo st=$?`, "zsh:1: nope: parameter not set\nst=1\n"},
		// And it is the resolved name for that expansion only.
		{`(n=(a b); print ${(P)n}.; setopt nounset; print $zz); echo st=$?`, ".\nzsh:1: zz: parameter not set\nst=1\n"},
		// The control: a resolved name that is set.
		{`setopt nounset; n=(a b); a=1; print ${(P)n}`, "1\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
