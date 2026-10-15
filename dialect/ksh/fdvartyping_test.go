// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// TestAFdVariableStoreStartsTheNameOver — a `{name}` store makes the name a
// plain integer whatever it was: every other attribute goes, its kind goes,
// and its export goes. The ksh93 reading of
// interp.Semantics.FdVariableStoreTyping (#6550).
//
// Measured 2026-10-15 with `-c` on ksh93u+ 2012-08-01; the rows are its
// output verbatim. The program rows are the restore: on a command that is a
// process of its own the store is the process's, so the name is as it was
// afterwards — and the program's environment has no `v` at all, which is the
// ksh93 column of #6521.
func TestAFdVariableStoreStartsTheNameOver(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a fresh name", "true {v}</dev/null; typeset -p v", "typeset -i v=10\n"},
		{"set -a", "set -a; true {v}</dev/null; typeset -p v", "typeset -i v=10\n"},
		{"an exported name", "export v=keep; true {v}</dev/null; typeset -p v; /usr/bin/env | /usr/bin/grep -c ^v=", "typeset -i v=10\n0\n"},
		{"a cased scalar", "typeset -u v=abc; true {v}</dev/null; typeset -p v", "typeset -i v=10\n"},
		{"an array", "a=(1 2); true {a}</dev/null; typeset -p a", "typeset -i a=10\n"},
		{"a table", "typeset -A m; m[k]=1; true {m}</dev/null; typeset -p m", "typeset -i m=10\n"},
		{"through a reference", "typeset -n r=t; t=1; true {r}</dev/null; typeset -p r t", "typeset -n r=t\ntypeset -i t=10\n"},
		{"a local", "function f { typeset v=loc; true {v}</dev/null; typeset -p v; }; f; echo \"[${v-unset}]\"", "typeset -i v=10\n[unset]\n"},
		{"a program's environment", "export v=keep; /usr/bin/env {v}</dev/null | /usr/bin/grep -c ^v=; typeset -p v", "0\ntypeset -x v=keep\n"},
		{"a program leaves the letters", "typeset -u v=abc; /usr/bin/true {v}</dev/null; typeset -p v", "typeset -u v=ABC\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// A name the shell was born with is taken out of the environment by the
// store, not merely forgotten: forgetting what was said about its export
// falls back to the environment the shell started with, which would hand it
// on again. Measured 2026-10-15, `env v=keep ksh -c 'true {v}</dev/null;
// env | grep -c ^v='` prints 0 on ksh93u+.
func TestAnInheritedNameIsNotExportedAfterAnFdVariableStore(t *testing.T) {
	out, _, err := preset.CombinedThroughTheAliases(t, dialecttest.Base{
		Dir: t.TempDir(), Env: []string{"v=keep"},
	}, "true {v}</dev/null; /usr/bin/env | /usr/bin/grep -c ^v=")
	if err != nil {
		t.Fatal(err)
	}
	if out != "0\n" {
		t.Errorf("got %q, want the child to see no v", out)
	}
}
