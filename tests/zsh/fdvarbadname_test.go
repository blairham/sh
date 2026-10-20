// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestADigitLedWordThatIsNotANumberIsNotAnIdentifier — `{1a}` before a
// redirection is lexed as a descriptor variable here (a digit leads, because
// `{1}` is the positional parameter) and refused at the store, after the
// open, as an error that ends the shell the store is made in (#6522). The
// rows are zsh 5.9.2's, `-fc`, measured 2026-10-15.
func TestADigitLedWordThatIsNotANumberIsNotAnIdentifier(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		st              int
	}{
		{"the shell ends", "echo before; echo {1a}</dev/null; echo after", "before\nzsh:1: not an identifier: 1a\n", 1},
		{"an underscore", "echo {1_}>/dev/null; echo after", "zsh:1: not an identifier: 1_\n", 1},
		{"in a function, the shell ends", "f() { echo {1a}</dev/null; echo in; }; f; echo after", "f: not an identifier: 1a\n", 1},
		{"a subshell ends, the shell goes on", "( echo {1a}</dev/null; echo in ); echo \"after $?\"", "zsh:1: not an identifier: 1a\nafter 1\n", 0},
		{"always still runs", "{ echo {1a}</dev/null; } always { echo always }; echo after", "zsh:1: not an identifier: 1a\nalways\n", 1},
		{"on a program, the command fails", "/usr/bin/true {1a}</dev/null; echo \"st=$?\"", "zsh:1: not an identifier: 1a\nst=1\n", 0},
		{"the open comes first", "echo {1a}</nonexist; echo \"st=$?\"", "zsh:1: no such file or directory: /nonexist\nst=1\n", 0},
		{"a close is not a store", "echo {1a}>&-; echo \"st=$?\"", "zsh:1: parameter 1a does not contain a file descriptor\nst=1\n", 0},
		{"a number is a positional", "set -- a; echo {01}</dev/null; echo \"st=$? [${1:+set}]\"", "\nst=0 [set]\n", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != c.st {
				t.Errorf("got %q st=%d, want %q st=%d", out, st, c.want, c.st)
			}
		})
	}
}
