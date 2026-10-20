// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// TestAnFdVariableIsNotInTheProgramsEnvironment — a program is handed the
// environment from before its own `{name}` store, which is the No side of
// interp.Semantics.FdVariableReachesTheProgramsEnvironment (#6521).
//
// Measured 2026-10-15 with `-c` on bash 5.3.20:
//
//	set -a; /usr/bin/env {v}</dev/null | grep ^v=          nothing
//	export v=keep; /usr/bin/env {v}</dev/null | grep ^v=   v=keep
//
// The builtin row is the control: a store this shell keeps is in the
// environment of the next program it runs, `set -a` and all.
func TestAnFdVariableIsNotInTheProgramsEnvironment(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"set -a", "set -a; /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=\n", "0\n"},
		{"an exported name", "export v=keep; /usr/bin/env {v}</dev/null | /usr/bin/grep ^v=\n", "v=keep\n"},
		{"through command", "set -a; command /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=\n", "0\n"},
		{"two names", "set -a; /usr/bin/env {v}</dev/null {w}</dev/null | /usr/bin/grep -c -E '^(v|w)='\n", "0\n"},
		{"a prefix wins", "export v=keep; v=pre /usr/bin/env {v}</dev/null | /usr/bin/grep ^v=\n", "v=pre\n"},
		{"a builtin's store is the next program's", "set -a; true {v}</dev/null; /usr/bin/env | /usr/bin/grep -c ^v=\n", "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs := runBashSplit(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("out %q err %q, want %q", out, errs, c.want)
			}
		})
	}
}

// The hold a program's store is weighed against is that program's and no
// other's: a program that never ran because a later redirection failed
// leaves nothing for the next one, and the programs a not-found handler runs
// are handed environments of their own — bash shows them the store.
//
// Measured 2026-10-15 with `-c` on bash 5.3.20; the rows are its output.
func TestAnFdVariableHoldIsOneProgramsOwn(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a program that did not run",
			"export v=keep; /usr/bin/env {v}</dev/null 3</nonexist 2>/dev/null; v=new; /usr/bin/env >e; /usr/bin/grep ^v= e\n",
			"v=new\n",
		},
		{
			"a substitution in the program's own redirection",
			"set -a; /usr/bin/true {v}</dev/null 3<$(/usr/bin/env >e; echo /dev/null); /usr/bin/grep -c ^v= e\n",
			"1\n",
		},
		{
			"a not-found handler's program",
			"export v=keep; command_not_found_handle() { /usr/bin/env | /usr/bin/grep -c ^v=keep; }; nosuch {v}</dev/null\n",
			"0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBashSplit(t, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
