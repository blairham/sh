// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestKshZeroSubscriptNamesTheFirstElement pins `ksh_zero_subscript`: a
// subscript of 0, or a pair of them, names the first element, and nothing
// else moves (#5152). Measured 2026-10-02 on zsh 5.9.2
// (`/opt/homebrew/bin/zsh -f`, `LC_ALL=C`).
func TestKshZeroSubscriptNamesTheFirstElement(t *testing.T) {
	const setup = "a=(p q r s); s=hello\n"
	for _, tc := range []struct{ src, want string }{
		// Off, which is the default: element 0 is no element.
		{`print -r -- :$a[0]:${a[0,0]}:`, ":::\n"},
		{`setopt kshzerosubscript; print -r -- :$a[0]:${a[-0]}:$s[0]:${a[0]:-d}:${+a[0]}:`, ":p:p:h:p:1:\n"},
		{`setopt kshzerosubscript; a[0]=W; print -r -- $a`, "W q r s\n"},
		{`setopt kshzerosubscript; b=(); b[0]=n; print -r -- $b ${#b}`, "n 1\n"},
		// A pair moves only where both ends are 0.
		{`setopt kshzerosubscript; print -r -- :${a[0,0]}:${a[1,0]}:${a[0,1]}:${a[2,0]}:${a[0,2]}:`, ":p::p::p q:\n"},
		{`setopt kshzerosubscript; a[0,0]=W; print -r -- $a`, "W q r s\n"},
		{`setopt kshzerosubscript; a[1,0]=W; print -r -- $a`, "W p q r s\n"},
		// Arithmetic is not reached.
		{`setopt kshzerosubscript; print -r -- $((a[0]))`, "0\n"},
		// The backward miss is the index 0, read and written as the first.
		{`setopt kshzerosubscript; print -r -- :$a[(R)nf]:$a[(I)nf]:$s[(R)z]:$a[(r)nf]:`, ":p:0:h::\n"},
		{`setopt kshzerosubscript; a[(R)nf]=X; print -r -- $a`, "X q r s\n"},
		{`setopt kshzerosubscript; unsetopt kshzerosubscript; print -r -- :$a[0]:`, "::\n"},
		{`setopt kshzerosubscript; [[ -o kshzerosubscript ]] && print on`, "on\n"},
	} {
		got, _ := runZsh(t, t.TempDir(), setup+tc.src)
		if got != tc.want {
			t.Errorf("%s\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
