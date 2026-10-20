// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// **A double-quoted expansion in a delimiter keeps its quotes** —
// syntax.Dialect.HeredocDelimiterKeepsAQuotedExpansion. Measured 2026-10-03
// on ksh93u+: `<<"$d"` ends at a line reading `"$d"` and not at `$d`, where
// `<<"EOF"` ends at `EOF` and `<<\$d` at `$d`.
func TestADelimitersQuotedExpansionKeepsItsQuotes(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"while read -r l; do print -r -- \"[$l]\"; done <<\"$d\"\n$d\n\"$d\"\n", "[$d]\n"},
		{"while read -r l; do print -r -- \"[$l]\"; done <<a\"${d}\"\nx\na\"${d}\"\n", "[x]\n"},
		{"while read -r l; do print -r -- \"[$l]\"; done <<\"EOF\"\nx\nEOF\n", "[x]\n"},
		{"while read -r l; do print -r -- \"[$l]\"; done <<\\$d\nx\n$d\n", "[x]\n"},
	} {
		out, _ := runKsh(t, t.TempDir(), c.src)
		if out != c.want {
			t.Errorf("%q: got %q, want %q", c.src, out, c.want)
		}
	}
}
