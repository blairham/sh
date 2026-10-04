// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// Once a member matches, the rest of the bracket is skipped to the next `]`
// byte, an escaped one included. Measured 2026-10-03 on dash.
func TestAMatchedBracketSkipsToAnEscapedBracket(t *testing.T) {
	for _, c := range []struct{ pat, want string }{
		{`[a\]b]`, "[]][b]"},
		{`[a\]]`, "[]]"},
		{`[\]a]`, "[a][]]"},
		{`[a\]b]c`, "[]c][bc]"},
		{`[!a\]b]`, "[c][x]"},
		{`[a"]"b]`, "[]][b]"},
		// The control: no escaped bracket, so nothing changes.
		{`[ab]`, "[a][b]"},
	} {
		src := `for s in a ']' b c x ']c' ac bc; do case $s in ` + c.pat + `) printf '[%s]' "$s";; esac; done`
		if out, _ := runDash(t, t.TempDir(), src); out != c.want {
			t.Errorf("%s: %q, want %q", c.pat, out, c.want)
		}
	}
}
