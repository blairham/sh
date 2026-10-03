// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAPairOnAParameterReferenceIsARange pins that a written pair after a
// `(P)` reference reads a range of what the reference names, where it was
// read as the arithmetic comma (#5151, a chunk of D04parameter.ztst).
// Measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestAPairOnAParameterReferenceIsARange(t *testing.T) {
	const setup = "abc=quick; foo=abc; bar=abcdef; a=(p q r s); v=a; w='a[@]'\n"
	for _, tc := range []struct{ src, want string }{
		{`print -r -- ${${(P)foo}[1,3]} ${${(P)bar[1,3]}[1,3]}`, "qui qui\n"},
		{`print -r -- ${${(P)v}[2,3]} ${#${(P)v}[2,3]} ${${(P)w}[2,3]}`, "q r 2 q r\n"},
		{`print -r -- ${${(P)foo}[(r)q*,3]}`, "qui\n"},
		// The control: one subscript was always right.
		{`print -r -- ${${(P)foo}[2]}`, "u\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
