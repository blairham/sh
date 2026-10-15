// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// TestAFdVariableStoreMakesANewNameAnInteger — a `{name}` store gives the
// integer attribute to a name that is not already holding a scalar, and
// leaves an existing scalar's attributes as they were. The zsh reading of
// interp.Semantics.FdVariableStoreTyping (#6550).
//
// Measured 2026-10-15 with `-fc` on zsh 5.9.2; the rows are its output
// verbatim. The program rows are the restore: on a command that is a process
// of its own the store is the process's, so the name — letters and all — is
// as it was afterwards.
func TestAFdVariableStoreMakesANewNameAnInteger(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a fresh name", "true {v}</dev/null; typeset -p v", "typeset -i v=11\n"},
		{"set -a", "set -a; true {v}</dev/null; typeset -p v", "export -i v=11\n"},
		{"an existing scalar", "v=x; true {v}</dev/null; typeset -p v", "typeset v=11\n"},
		{"a cased scalar", "typeset -u v=abc; true {v}</dev/null; typeset -p v", "typeset -u v=11\n"},
		{"an array", "a=(1 2); true {a}</dev/null; typeset -p a", "typeset -i a=11\n"},
		{"a local array", "f() { local -a v; true {v}</dev/null; typeset -p v; }; f; echo \"[${v-unset}]\"", "typeset -i v=11\n[unset]\n"},
		{"a later string", "true {v}</dev/null; v=abc; typeset -p v", "typeset -i v=0\n"},
		{"a program leaves a fresh name unset", "/usr/bin/true {v}</dev/null; echo \"[${v-unset}]\"", "[unset]\n"},
		{"a program leaves the letters", "typeset -u v=abc; /usr/bin/true {v}</dev/null; typeset -p v", "typeset -u v=abc\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src)
			if out != c.want || st != 0 {
				t.Errorf("got %q st=%d, want %q", out, st, c.want)
			}
		})
	}
}
