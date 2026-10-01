// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A module parameter withdrawn while a local stands over it stays withdrawn
// once the function returns (#5158). The local had set the producer, the
// freeze and the value-hiding mark aside to put back on its way out, so a
// withdrawal that captured only the tables as they stood captured nothing,
// and the scope then restored the module's parameter after the module had
// given it up. Measured 2026-09-30 on zsh 5.9.2 (`-f`, a script file under
// `env -i PATH=/usr/bin:/bin LC_ALL=C`); each row is the line that shell
// wrote.
func TestAWithdrawalUnderALocalOutlivesTheLocal(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{
			"a first load refused by a local leaves the name unset",
			`f() { local EPOCHSECONDS=mine; zmodload zsh/datetime 2>/dev/null; print -rn -- "$? $EPOCHSECONDS " }; f; print -rn -- "${+EPOCHSECONDS}"`,
			"2 mine 0",
		},
		{
			"and the feature can then be selected, and produces",
			`f() { local EPOCHSECONDS=mine; zmodload zsh/datetime 2>/dev/null }; f; zmodload -F zsh/datetime p:EPOCHSECONDS; print -rn -- "$? ${+EPOCHSECONDS} ${(t)EPOCHSECONDS}"`,
			"0 1 integer-readonly-hide-hideval-special",
		},
		{
			"a deselection under a local",
			`zmodload zsh/datetime; f() { local EPOCHSECONDS=5; zmodload -F zsh/datetime -p:EPOCHSECONDS; print -rn -- "$EPOCHSECONDS " }; f; print -rn -- "${+EPOCHSECONDS} "; zmodload -F zsh/datetime +p:EPOCHSECONDS; print -rn -- "$? ${+EPOCHSECONDS}"`,
			"5 0 0 1",
		},
		{
			"an unload under a local leaves an ordinary name behind",
			`zmodload zsh/datetime; g() { local EPOCHSECONDS=x; zmodload -u zsh/datetime }; g; EPOCHSECONDS=(a b); print -rn -- "$? ${(t)EPOCHSECONDS} $EPOCHSECONDS"`,
			"0 array a b",
		},
		// The control: the same unload with no local was already right, and
		// must not move.
		{
			"an unload with no local",
			`zmodload zsh/datetime; zmodload -u zsh/datetime; EPOCHSECONDS=(a b); print -rn -- "$? ${(t)EPOCHSECONDS} $EPOCHSECONDS"`,
			"0 array a b",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, st := runZsh(t, dir, c.src+"\n"); out != c.want || st != 0 {
				t.Errorf("got %q at %d, want %q", out, st, c.want)
			}
		})
	}
}
