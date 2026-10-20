// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A body line that only starts the delimiter keeps its continuation** —
// syntax.Dialect.HeredocPrefixLineKeepsItsContinuation. Measured 2026-10-03 on
// ksh93u+ with delimiter ABC: `AB\` over `X` and `A\` over `BC` are each two
// lines with the backslash kept, `ABC\` counts as a start, and `B\` and `xA\`
// join as any other line does.
func TestABodyLineStartingTheDelimiterKeepsItsContinuation(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"AB\\\nX\n", "[AB\\]\n[X]\n"},
		{"A\\\nBC\n", "[A\\]\n[BC]\n"},
		{"ABC\\\nX\n", "[ABC\\]\n[X]\n"},
		{"B\\\nX\n", "[BX]\n"},
		{"xA\\\nX\n", "[xAX]\n"},
		{"\\\nX\n", "[X]\n"},
	} {
		src := "while IFS= read -r l; do print -r -- \"[$l]\"; done <<ABC\n" + c.body + "ABC\n"
		out, st := runKsh(t, t.TempDir(), src)
		if st != 0 || out != c.want {
			t.Errorf("%q: got %q at %d, want %q", c.body, out, st, c.want)
		}
	}
}
