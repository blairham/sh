// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// TestAnFdVariableBeforeBothStreams — a `{name}` in front of `&>` and its
// family is a descriptor variable here: the file is opened on a number the
// name receives, standard error goes there with it and standard output is
// left alone, and one more number is held while the name's is picked
// (#6665). zsh 5.9.2 `-fc`, measured 2026-10-15.
func TestAnFdVariableBeforeBothStreams(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"the name", `echo hi {v}&>/dev/null; echo "[$v]"`, "hi\n[12]\n"},
		{"across a blank", `echo hi {v} &>/dev/null; echo "[$v]"`, "hi\n[12]\n"},
		{"an append", `echo hi {v}&>>/dev/null; echo "[$v]"`, "hi\n[12]\n"},
		{"the clobber marks", `: {v}&>|/dev/null; : {w}&>!/dev/null; echo "[$v][$w]"`, "[12][13]\n"},
		{"the csh spelling", `: {v}>&/dev/null; echo "[$v]"`, "[12]\n"},
		{"the held number comes free", `: {v}&>/dev/null {w}</dev/null; echo "$v $w"`, "12 11\n"},
		{"standard error goes with it", `print -u2 err {v}&>f; print -u2 after; read l <f; echo "f:$l"`, "after\nf:err\n"},
		{"on exec", `exec 3>&2 {v}&>f; print -u2 err; print -u$v more; exec 2>&3; echo "[$v]"; while read l; do echo "f:$l"; done <f`, "[12]\nf:err\nf:more\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestADoubledAngleOpensAProcessSubstitution — `>>(` and `<<(` are a
// one-character operator in front of a process substitution here, not an
// append or a here-document (#6665). zsh 5.9.2 `-fc`, measured 2026-10-15.
func TestADoubledAngleOpensAProcessSubstitution(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a read", `read l <<(echo hi); echo "[$l]"`, "[hi]\n"},
		{"two of them", `while read l; do echo "[$l]"; done <<(echo a)<<(echo b)`, "[a]\n[b]\n"},
		{"numbered", `read l 3<<(echo hi) <&3; echo "[$l]"`, "[hi]\n"},
		{"a write", `echo hi >>(read l; echo "[$l]")`, "[hi]\n"},
		{"a {name} before it", `echo hi {v}>>(read l); echo "${v:+set}"`, "hi\nset\n"},
		{"the both-streams append is not this", `echo hi &>>(cat)`, "zsh:1: number expected\n"},
		{"a here-string is not this", `read l <<<(x); echo "[$l]"`, "[(x)]\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), c.src)
			if out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// TestAnFdVariableBeforeBothStreamsIsAWordInALiteral — where a `{name}` is
// not read across blanks it is not read in front of `&>` either: an array
// literal keeps `{v}` an element and refuses the operator, “parse error near
// `&>'“ in zsh 5.9.2, measured 2026-10-15 (#6665).
func TestAnFdVariableBeforeBothStreamsIsAWordInALiteral(t *testing.T) {
	for _, src := range []string{"a=({v}&>/dev/null)", "a=(x {v}&>/dev/null)"} {
		_, err := syntax.Parse(src, zsh.Dialect())
		if err == nil || !strings.Contains(err.Error(), `"&>" unexpected`) {
			t.Errorf("%q: err %v, want the operator refused", src, err)
		}
	}
}
