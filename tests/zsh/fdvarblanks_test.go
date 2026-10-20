// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// TestAnFdVariableReachesItsOperatorAcrossBlanks — a `{name}` among a
// command's arguments is a descriptor variable here when only blanks stand
// between it and a redirection operator that has no number of its own
// (#6587). zsh 5.9.2 `-fc`, measured 2026-10-15; bash and ksh93 need the
// operator adjacent.
func TestAnFdVariableReachesItsOperatorAcrossBlanks(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a blank", `echo {v} >/dev/null; echo "[$v]"`, "\n[11]\n"},
		{"several", `echo {v}   </dev/null; echo "[$v]"`, "\n[11]\n"},
		{"a tab", "echo {v}\t>/dev/null; echo \"[$v]\"", "\n[11]\n"},
		{"a continued line", "echo {v} \\\n>/dev/null; echo \"[$v]\"", "\n[11]\n"},
		{"an append", `echo {v} >>/dev/null; echo "[$v]"`, "\n[11]\n"},
		{"a duplication", `echo {v} >&2 2>/dev/null; echo "[$v]"`, "\n[11]\n"},
		{"a here-string", `echo {v} <<<x; echo "[$v]"`, "\n[11]\n"},
		{"after exec", `exec {v} </dev/null; echo "[$v]"`, "[11]\n"},
		{"a later argument", `echo a {v} >/dev/null; echo "[$v]"`, "a\n[11]\n"},
		{"the refused spelling too", `echo {1a} >/dev/null; echo after`, "zsh:1: not an identifier: 1a\n"},
		{"a number of its own is an argument", `echo {v} 2>/dev/null; echo "[$v]"`, "{v}\n[]\n"},
		{"a word between", `echo {v} x >/dev/null; echo "[$v]"`, "[]\n"},
		{"only the last word", `echo {v} {w} >/dev/null; echo "[$v][$w]"`, "{v}\n[][11]\n"},
		{"quoted", `echo "{v}" >/dev/null; echo "[$v]"`, "[]\n"},
		{"a process substitution", `echo {v} <(:) >/dev/null; echo "[$v]"`, "[]\n"},
		{"a nameless function's words", `() { echo in; } {v} >/dev/null; echo "[$v]"`, "[]\n"},
		{"inside a substitution", `x=$(echo {w} >/dev/null; echo "[$w]"); echo "$x"`, "\n[11]\n"},
		{"after a substitution", `echo $(true) {v} >/dev/null; echo "[$v]"`, "\n[11]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// And a `for` list is not a command's arguments: the blank form stays a word
// there and the operator after it is the parse error the reference reports,
// “parse error near `>'“, not one about a descriptor variable.
//
// Nor is the position before the next command's word, where the `{` opens a
// brace group: `echo a; {v} >/dev/null echo hi` is “parse error near
// `echo'“ in the reference, as the same command alone is.
func TestAnFdVariableAcrossBlanksIsNotAForListsWord(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"for i in {v} >/dev/null; do :; done", `">" unexpected`},
		{"echo a; {v} >/dev/null echo hi", `"echo" unexpected`},
	} {
		_, err := syntax.Parse(c.src, zsh.Dialect())
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: err %v, want %s", c.src, err, c.want)
		}
	}
}
