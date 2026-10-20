// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// `\w` here reads the `PWD` **parameter**, so a script that writes over it
// moves what the prompt draws — #4540.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/bash`, GNU bash 5.3.20, with
// `--norc`; `go version -m` says *not a Go executable* for it:
//
//	cd <d>; PWD=/bogus; PS1='\w'; echo "${PS1@P}"    /bogus
//	cd <d>;             PS1='\w'; echo "${PS1@P}"    <d>
//
// The second line is the control: both answers are reachable and only the
// assignment tells them apart. zsh is the other way round on the same shape
// and BusyBox ash agrees with zsh, which is why this is a field on the style
// rather than a rule in the walker — see
// interp.PromptStyle.CwdIsTheShellsOwnDirectory.
func TestAPromptReadsThePwdParameter(t *testing.T) {
	if bash.PromptStyle().CwdIsTheShellsOwnDirectory {
		t.Fatal("CwdIsTheShellsOwnDirectory is on, so a written PWD would not move this prompt")
	}
	dir := t.TempDir()
	if out, st := runBash(t, dir, `PWD=/bogus; PS1='\w'; echo "${PS1@P}"`); out != "/bogus\n" || st != 0 {
		t.Errorf(`\w after PWD=/bogus = %q (status %d), want "/bogus\n"`, out, st)
	}
	// The control, which is the same line without the assignment.
	if out, st := runBash(t, dir, `PS1='\w'; echo "${PS1@P}"`); out != dir+"\n" || st != 0 {
		t.Errorf(`\w with nothing written = %q (status %d), want %q`, out, st, dir+"\n")
	}
}
