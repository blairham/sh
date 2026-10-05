// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// Which calls the shell makes on its own account hold a job number, and
// whether they take one of their own inside a function that holds one
// (#5909).
//
// Measured 2026-10-05 on zsh 5.9.2 (/opt/homebrew/bin/zsh) under `-f`, each
// line with J standing for `sleep 1 & print -r -- ${(k)jobstates}`;
// the column is the first line J printed. A signal's trap and `cd`'s chpwd
// run while a command runs, and take a number even inside f; ZERR, DEBUG,
// and EXIT or zshexit where the script ran out, run between commands and
// take none; EXIT and zshexit after an `exit` take one.
func TestTheShellsOwnCallsHoldJobNumbersAsZshDoes(t *testing.T) {
	for _, c := range []struct{ body, want string }{
		{"trap 'J' USR1; kill -USR1 $$; :", "2"},
		{"TRAPUSR1(){ J }; kill -USR1 $$; :", "2"},
		{"f(){ trap 'J' USR1; kill -USR1 $$; : }; f", "3"},
		{"TRAPUSR1(){ J }; f(){ kill -USR1 $$; : }; f", "3"},
		{"chpwd(){ J }; cd /", "2"},
		{"chpwd(){ J }; f(){ cd / }; f", "3"},
		{"TRAPZERR(){ J }; false", "1"},
		{"TRAPDEBUG(){ J }; :", "1"},
		{"TRAPEXIT(){ J }", "1"},
		{"TRAPEXIT(){ J }; exit", "2"},
		{"trap 'J' EXIT; exit", "2"},
		{"trap 'J' EXIT; f(){ exit }; f", "2"},
		{"zshexit(){ J }", "1"},
		{"zshexit(){ J }; exit", "2"},
		// The controls: a function called from a function takes nothing of
		// its own, and an `if` inside one does.
		{"g(){ J }; f(){ g }; f", "2"},
		{"f(){ if true; then J fi }; f", "3"},
	} {
		t.Run(c.body, func(t *testing.T) {
			scratchHome(t)
			var o, e bytes.Buffer
			sh := scratchShell(t)
			sh.Stdout, sh.Stderr = &o, &e
			sh.Stdin = strings.NewReader("")
			// Written out in place rather than through an alias, which a
			// string read whole before it runs would not see; and the
			// background job's streams on the null device, so its output
			// is not a second writer into the buffer the shell writes to.
			src := "zmodload zsh/parameter\n" + strings.ReplaceAll(c.body, "J", "sleep 0.3 >/dev/null 2>&1 & print -r -- R=${(k)jobstates};") + "\n"
			driver.MainArgs(sh, []string{"zsh", "-f", "-c", src})
			first, _, _ := strings.Cut(o.String(), "\n")
			if want := "R=" + c.want; first != want {
				t.Errorf("first line %q, want %q (all %q, stderr %q)", first, want, o.String(), e.String())
			}
		})
	}
}
