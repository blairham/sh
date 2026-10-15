// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// TestABraceWordThatIsNotANameIsRefused — any brace word directly before a
// redirection operator is a descriptor variable here, and one that is not a
// name is refused before anything else about the redirection is done: the
// open, the close, the duplication (#6522). The rows are ksh93u+
// 2012-08-01's output, `-c`, measured 2026-10-15.
func TestABraceWordThatIsNotANameIsRefused(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a hyphen", `echo {a-b}</dev/null; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"a positional", `echo {1}</dev/null; echo "st=$?"`, "ksh: 1: invalid variable name\nst=1\n"},
		{"digits first", `echo {1a}>/dev/null; echo "st=$?"`, "ksh: 1a: invalid variable name\nst=1\n"},
		{"unexpanded", `echo {$x}</dev/null; echo "st=$?"`, "ksh: $x: invalid variable name\nst=1\n"},
		{"two dots", `echo {a..b}</dev/null; echo "st=$?"`, "ksh: a..b: invalid variable name\nst=1\n"},
		{"before the open", `echo {a-b}</nonexist; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"a close", `echo {a-b}>&-; echo "st=$?"`, "ksh: a-b: invalid variable name\nst=1\n"},
		{"in a function, the function goes on", `f() { echo {a-b}</dev/null; echo in; }; f; echo "after $?"`, "ksh: a-b: invalid variable name\nin\nafter 0\n"},
		{"on exec, the shell ends", `exec {1}</dev/null; echo after`, "ksh: 1: invalid variable name\n"},
		{"a comma is brace expansion", `echo {a,b}</dev/null`, "a b\n"},
		{"a name", `echo {_1}</dev/null; echo "st=$?"`, "\nst=0\n"},
		{"a dotted name", `echo {.a}</dev/null; echo "st=$?"`, "\nst=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
