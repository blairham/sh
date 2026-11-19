// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// A child a `( … )` starts and a signal kills is reported as that child,
// except the command the parentheses' fork would have exec'd, where the fork is
// the child and the notice names the parentheses (#7310, #7420).
//
// Measured 2026-11-19 under `-c`, the shell in a process group of its own,
// against GNU bash 5.3.20(1)-release, with K standing for `/bin/sh -c 'kill
// -USR1 $$'`: see interp.Runner.recordKilled for the table. This shell named the
// outer parentheses on every row whose command is not one.
func TestAChildASubshellStartsIsNamedInItsNotice(t *testing.T) {
	const k = `/bin/sh -c 'kill -USR1 $$'`
	usr1 := strconv.Itoa(128 + int(syscall.SIGUSR1))
	died := "st=" + usr1 + "\n"
	for _, tc := range []struct{ name, src, command, stdout string }{
		{"the tail", `( ` + k + ` ); echo st=$?`, `( ` + k + ` )`, died},
		{"the tail of an and-or", `( true && ` + k + ` ); echo st=$?`, `( true && ` + k + ` )`, died},
		{"not the tail", `( ` + k + `; echo x ); echo st=$?`, k, "x\nst=0\n"},
		{"the tail inside an if", `( if true; then ` + k + `; fi ); echo st=$?`, k, died},
		{"the tail inside a group", `( { ` + k + `; } ); echo st=$?`, k, died},
		{"a trap keeps the fork", `( trap 'echo t' USR2; ` + k + ` ); echo st=$?`, k, died},
		{"inner parentheses", `( ( ` + k + ` ); echo x ); echo st=$?`, `( ` + k + ` )`, "x\nst=0\n"},
		{
			"inner parentheses killing themselves",
			`( ( kill -USR1 $BASHPID ); echo x=$? ); echo st=$?`, `( kill -USR1 $BASHPID )`,
			"x=" + usr1 + "\nst=0\n",
		},
		{
			"inside a job",
			`( ( kill -USR1 $BASHPID ); echo x ) & wait $!; echo st=$?`, `( kill -USR1 $BASHPID )`,
			"x\nst=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashCWithin(t, tc.src)
			if out != tc.stdout {
				t.Errorf("stdout = %q, want %q", out, tc.stdout)
			}
			if n := strings.Count(errs, "\n"); n != 1 {
				t.Fatalf("stderr = %q, want one notice and not %d lines", errs, n)
			}
			line := killedNoticeLine(t, errs, tc.command)
			if !strings.HasPrefix(line, "bash: line 1: ") {
				t.Errorf("the notice is %q, want the located form", line)
			}
		})
	}
}
