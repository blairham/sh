// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Two declaration rows over the shell's own parameters. An array letter with
// a plain word is an inconsistent type in a function as at the top, and a
// valueless local of the two history sizes holds the outer value where the
// shell's other integers start at 0. Measured 2026-10-03 on zsh 5.9.2 under
// `env -i PATH=/usr/bin:/bin`, `-f` (#5605).
func TestTwoDeclarationRowsOverTheShellsOwnParameters(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`f(){ local -a path=/x; }; f; echo after`, "f:local: path: inconsistent type for assignment\n"},
		{`f(){ local -a x=/y; }; f; echo after`, "f:local: x: inconsistent type for assignment\n"},
		{`f(){ typeset -a fpath=/y; }; f; echo after`, "f:typeset: fpath: inconsistent type for assignment\n"},
		{`f(){ local -a path=(); local path2=/s; print $#path $path2; }; f`, "0 /s\n"},
		{`f(){ local path=/s; print $#path; }; f`, "1\n"},
		{`HISTSIZE=50 SAVEHIST=7; f(){ local HISTSIZE SAVEHIST COLUMNS; print $HISTSIZE $SAVEHIST $COLUMNS; }; f; print $HISTSIZE`, "50 7 0\n50\n"},
		{`f(){ local HISTSIZE=5; print $HISTSIZE; }; f; print $HISTSIZE`, "5\n30\n"},
	} {
		if out, _ := runZsh(t, t.TempDir(), c.src); out != c.want {
			t.Errorf("%s\n got %q\nwant %q", c.src, out, c.want)
		}
	}
}
