// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/interp"
)

// TestRestrictedModeRefusesInItsOwnWords pins zsh's restricted mode. Measured
// 2026-10-02 on zsh 5.9.2 under `-f` (#5155); each row is one refusal, with
// `print after $?` behind it to show whether the script goes on.
func TestRestrictedModeRefusesInItsOwnWords(t *testing.T) {
	const on = "setopt restricted; "
	cases := []struct{ src, want string }{
		{"set -r; cd /; print after $?", "zsh:cd:1: restricted\nafter 1\n"},
		{on + "PATH=/x; print after $?", "zsh:1: PATH: restricted\n"},
		{on + "path=(/x); print after $?", "zsh:1: path: restricted\n"},
		{on + "ENV=x; print after $?", "after 0\n"},
		{on + "unset SHELL; print after $?", "zsh:unset:1: SHELL: restricted\n"},
		{on + "typeset PATH=/x; print after $?", "zsh:typeset:1: PATH: restricted\n"},
		{on + "/bin/ls; print after $?", "zsh:1: /bin/ls: restricted\nafter 1\n"},
		{on + "print a >f; print after $?", "zsh:1: writing redirection not allowed in restricted mode\nafter 1\n"},
		{on + "exec ls; print after $?", "zsh:exec:1: ls: restricted\n"},
		{on + "command -p echo hi; print after $?", "zsh:1: echo: restricted\nafter 1\n"},
		{on + "hash ls=zz; print after $?", "zsh:hash:1: restricted: zz\nafter 1\n"},
		{on + "hash -d foo=/tmp; print after $?", "zsh:hash:1: restricted: /tmp\nafter 1\n"},
		{on + "commands[ls]=zz; print after $?", "zsh:1: restricted: zz\nafter 0\n"},
		{on + "unsetopt restricted; print after $?", "zsh:unsetopt:1: can't change option: restricted\nafter 1\n"},
		{on + "set +r; print after $?", "zsh:set:1: can't change option: -r\n"},
		{on + "f(){ setopt localoptions restricted; }; f; [[ -o restricted ]] && print still", "still\n"},
	}
	for _, c := range cases {
		if got, _ := runZshRoute(t, t.TempDir(), c.src, interp.RouteCommandString); got != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
		}
	}
}

// And `.` on a path is taken in this mode, where the other two refuse it.
func TestRestrictedModeReadsADotPath(t *testing.T) {
	dir := t.TempDir()
	got, _ := runZsh(t, dir, "print 'print sourced' >| x\nsetopt restricted\n. ./x\nprint after $?\n")
	if want := "sourced\nafter 0\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
