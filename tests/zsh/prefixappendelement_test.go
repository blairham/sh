// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A scalar `+=` written as a prefix over an array adds an element, as the
// statement does here, and an array reaches no child's environment. We
// joined the value onto the array's text and handed every command a scalar
// (#6751).
//
// Measured 2026-10-16 on zsh 5.9.2 `-f` from a file, byte for byte.
func TestAPrefixAppendOverAnArrayAddsAnElement(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ src, want string }{
		{"a=(x y); a+=z /usr/bin/printenv a; echo st=$?", "st=1\n"},
		{"a=(x y); f(){ typeset -p a; }; a+=z f; typeset -p a", "typeset -g -a a=( x y z )\ntypeset -a a=( x y )\n"},
		{`a=(x y); a+=z eval "typeset -p a"; typeset -p a`, "typeset -a a=( x y z )\ntypeset -a a=( x y )\n"},
		{`a=(x y); a+=z b=$a eval "typeset -p b"`, "typeset b='x y z'\n"},
		{`s=x; s+=z eval "typeset -p s"`, "typeset s=xz\n"},
	} {
		if out, _ := answersRun(t, c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
