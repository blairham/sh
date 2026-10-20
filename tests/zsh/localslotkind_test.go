// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A fresh local of one of the shell's own parameters takes the kind of the
// slot it shadows: an array slot's local is the array of a scalar value, and
// an integer slot's local evaluates its value. Measured 2026-10-03 on zsh
// 5.9.2 under `env -i PATH=/usr/bin:/bin`, `-f` (#5598, #5576). The last rows
// are the controls: `-h`, a kind letter on the line, and `SECONDS`, which is
// produced and an integer local either way.
func TestALocalOfTheShellsOwnSlotTakesItsKind(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`f(){ local path=/somewhere; print ${#path} ${(t)path} $PATH; }; f`, "1 array-local-tied-special /somewhere\n"},
		{`f(){ local fpath="/a /b"; print ${#fpath}; }; f`, "1\n"},
		{`f(){ typeset path=/q; print ${#path} ${(t)path}; }; f`, "1 array-local-tied-special\n"},
		{`f(){ local argv=x; print ${(t)argv} $#argv; }; f a b`, "array-local-special 1\n"},
		{`f(){ local SHLVL=4 HISTSIZE=3; print ${(t)SHLVL} ${(t)HISTSIZE}; }; f`, "integer-local-special integer-local-special\n"},
		{`f(){ local COLUMNS=1+1 SHLVL=abc; print $COLUMNS $SHLVL; typeset -p COLUMNS; }; f`, "2 0\ntypeset -i10 COLUMNS=2\n"},
		{`f(){ local SHLVL=4; unset SHLVL; SHLVL=2+3; print ${(t)SHLVL} $SHLVL; }; f`, "integer-local-special 5\n"},
		{`f(){ typeset LINES=5; print ${(t)LINES}; }; f`, "integer-local-special\n"},
		{`f(){ local -h path=/x; print ${(t)path} $#path; }; f`, "scalar-local-hide 2\n"},
		{`f(){ local -i SHLVL=3; typeset -p SHLVL; }; f`, "typeset -i SHLVL=3\n"},
		{`f(){ local SECONDS=3; print ${(t)SECONDS}; }; f`, "integer-local-special\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q", c.src, out, st, c.want)
		}
	}
}
