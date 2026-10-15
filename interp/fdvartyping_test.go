// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// The zero value of Semantics.FdVariableStoreTyping is an ordinary
// assignment, which is what every dialect did before the axis existed and is
// still bash's: the name keeps its letters and its export, and nothing is
// asked. The two other readings are pinned in their dialects' own tests,
// where the listings they change are spelled the way those shells spell them.
func TestAnFdVariableStoreIsAnAssignmentByDefault(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a fresh name is not an integer", `: {v}</dev/null; v=3+4; echo "$v"`, "3+4\n"},
		{"an integer stays one", `typeset -i v; : {v}</dev/null; v=3+4; echo "$v"`, "7\n"},
		{"an export stays", `export v=keep; : {v}</dev/null; env | grep -c '^v='`, "1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out := runFdVar(t, t.TempDir(), nil, c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// The other two readings, by value rather than by listing, through the core:
// what each does to a later string and to a child's environment.
func TestAnFdVariableStoreRetypes(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		typing          FdVariableStoreTypingPolicy
	}{
		{"new name: a fresh name evaluates", `: {v}</dev/null; v=3+4; echo "$v"`, "7\n", FdVariableStoreMakesANewNameAnInteger},
		{"new name: a scalar does not", `v=x; : {v}</dev/null; v=3+4; echo "$v"`, "3+4\n", FdVariableStoreMakesANewNameAnInteger},
		{"new name: the export stays", `export v=keep; : {v}</dev/null; env | grep -c '^v='`, "1\n", FdVariableStoreMakesANewNameAnInteger},
		{"start over: a scalar evaluates", `v=x; : {v}</dev/null; v=3+4; echo "$v"`, "7\n", FdVariableStoreStartsTheNameOver},
		{"start over: the export goes", `export v=keep; : {v}</dev/null; env | grep -c '^v='`, "0\n", FdVariableStoreStartsTheNameOver},
		{"start over: on a program, the export is back after", `export v=keep; cat {v}</dev/null </dev/null; env | grep -c '^v='`, "1\n", FdVariableStoreStartsTheNameOver},
		{"start over: on a program, the program has no v", `export v=keep; env {v}</dev/null | grep -c '^v='`, "0\n", FdVariableStoreStartsTheNameOver},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runFdVar(t, t.TempDir(), func(s *Semantics) {
				s.FdVariableStoreTyping = c.typing
			}, c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
