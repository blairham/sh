// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/driver"
)

// **A TRAP<signal> function's non-zero return interrupts the shell** (#5359).
// Measured 2026-10-02 on zsh 5.9.2, each row both as a `-c` string and as a
// script file. See interp/trapinterrupt.go.
func TestATrapFunctionsReturnInterruptsTheShell(t *testing.T) {
	const tr = `TRAPUSR1() { return 3 }; `
	for _, c := range []struct {
		name, src        string
		cOut, fOut       string
		cStatus, fStatus int
	}{
		{"the rest does not run", tr + `kill -USR1 $$; print no`, "", "", 0, 1},
		{"nor the rest of a function", tr + `f() { kill -USR1 $$; print f }; f; print no`, "", "", 0, 1},
		{"nor of a loop", tr + `for i in 1 2; do kill -USR1 $$; print $i; done; print no`, "", "", 0, 1},
		{"nor of an and-list", tr + `kill -USR1 $$ && print and; print no`, "", "", 0, 1},
		{"the EXIT trap, from a file only", `trap 'print X' EXIT; ` + tr + `kill -USR1 $$; print no`, "", "X\n", 0, 1},
		{"after a subshell, the handler's status", tr + `( kill -USR1 $$; print sub ); print no`, "sub\n", "sub\n", 3, 3},
		// A signal a program sends arrives through the runtime on a goroutine of
		// its own, so a row for `/bin/kill` would be graded on scheduling; the
		// subshell row above reaches the same origin deterministically.
		{"from a substitution, 1", tr + `x=$(kill -USR1 $$; print sub); print no`, "", "", 1, 1},
		{"eval fails", tr + `eval 'kill -USR1 $$; print e'; print no`, "", "", 1, 1},
		{
			"an always half runs and fails",
			tr + `{ kill -USR1 $$; print in } always { print always $? }; print no`,
			"always 0\n", "always 0\n", 1, 1,
		},
		{
			"in a function too",
			tr + `f() { { kill -USR1 $$ } always { print A $? } }; f; print no`,
			"A 0\n", "A 0\n", 1, 1,
		},
		{"a dot gives up its file and returns 126", tr + `. ./src; print after $?`, "after 126\n", "after 126\n", 0, 0},
		{"control: no return in the handler", `TRAPUSR1() { (exit 3) }; kill -USR1 $$; print yes`, "yes\n", "yes\n", 0, 0},
		{"control: a zero return", `TRAPUSR1() { return 0 }; kill -USR1 $$; print yes`, "yes\n", "yes\n", 0, 0},
		{"control: an action's return", `trap 'return 3' USR1; f() { kill -USR1 $$; print f }; f; print after $?`, "after 0\n", "after 0\n", 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if err := os.WriteFile(filepath.Join(dir, "src"), []byte("kill -USR1 $$\nprint insrc\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(dir, "s.zsh")
			if err := os.WriteFile(script, []byte(c.src+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, route := range []struct {
				args   []string
				out    string
				status int
			}{
				{[]string{"zsh", "-f", "-c", c.src}, c.cOut, c.cStatus},
				{[]string{"zsh", "-f", script}, c.fOut, c.fStatus},
			} {
				var out lockedOutput
				sh := zshShell()
				sh.Stdout, sh.Stderr = &out, &out
				st := driver.MainArgs(sh, route.args)
				if got := out.String(); got != route.out || st != route.status {
					t.Errorf("%v\ngot  %q, %d\nwant %q, %d", route.args[1:len(route.args)-1], got, st, route.out, route.status)
				}
			}
		})
	}
}
