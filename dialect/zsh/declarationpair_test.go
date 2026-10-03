// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestADeclarationsPairNamesASpan pins that a subscripted operand written as
// a pair replaces the span it names, as the plain assignment does, where the
// comma used to be read as the arithmetic operator (#5440). Measured
// 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestADeclarationsPairNamesASpan(t *testing.T) {
	const setup = "a=(p q r s); s=abcd\n"
	for _, tc := range []struct{ src, want string }{
		{`typeset "a[2,3]"=W; print -r -- "<$a>"`, "<p W s>\n"},
		{`typeset a[2,3]=W; print -r -- "<$a>"`, "<p W s>\n"},
		{`typeset "a[1,0]"=W; print -r -- "<$a>"`, "<W p q r s>\n"},
		{`typeset "a[3,9]"=W; print -r -- "<$a>"`, "<p q W>\n"},
		{`export "a[2,3]"=W; print -r -- "<$a>"`, "<p W s>\n"},
		{`typeset "s[2,3]"=XY; print -r -- "<$s>"`, "<aXYd>\n"},
		// The controls: one subscript spelled as a pair, and the refusal a
		// span wholly below the first element gets.
		{`typeset "a[2,2]"=W; print -r -- "<$a>"`, "<p W r s>\n"},
		{`typeset "a[0,0]"=W; print -r -- "<$a>"`, "zsh:2: a: assignment to invalid subscript range\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
