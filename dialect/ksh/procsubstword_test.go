// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A process substitution is a word of its own in ksh93u+, so what is written
// against it on either side is another word: an argument, a variable name for
// `read`, the command after an empty assignment. Measured 2026-10-16 on
// ksh93u+ 2012-08-01 under `-c` (#6813). See
// syntax.Dialect.ProcessSubstitutionIsAWordOfItsOwn.
func TestASubstitutionIsAWordOfItsOwn(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`f() { echo $#; }; f a<(true)b`, "3\n"},
		{`f() { echo $#; }; f <(true)<(true)`, "2\n"},
		{`read v < <(echo hi there)x; echo "[$v][$x]"`, "[hi][there]\n"},
		{`f() { echo $#; }; f "<(true)x"`, "1\n"},
	} {
		if out, _ := runKsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s: got %q, want %q", c.src, out, c.want)
		}
	}
}
