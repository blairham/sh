// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// The terminal and locale names are the shell's own once something sets them
// and absent until then, and an `unset` leaves the export letter behind for
// the next value. Measured 2026-10-03 on zsh 5.9.2 under `env -i
// PATH=/usr/bin:/bin`, `-f` (#5575). HISTFILE is the control: it is never
// special. See interp.Runner.MarkShellOwnParameterWhileSet.
func TestTheLocaleAndTerminalNamesAreTheShellsOwnWhileSet(t *testing.T) {
	src := `for n in TERM LANG LC_ALL RPS1 HISTFILE; do
  print -r -- "$n [${(tP)n}]"
  typeset $n=x; print -r -- "${(tP)n}"
  export $n=y; print -r -- "${(tP)n}"
  unset $n; print -r -- "[${(tP)n}]"
done
export TERM=x; unset TERM; TERM=y; /usr/bin/env | /usr/bin/grep '^TERM='`
	want := "TERM []\nscalar-special\nscalar-export-special\n[]\n" +
		"LANG []\nscalar-special\nscalar-export-special\n[]\n" +
		"LC_ALL []\nscalar-special\nscalar-export-special\n[]\n" +
		"RPS1 []\nscalar-special\nscalar-export-special\n[]\n" +
		"HISTFILE []\nscalar\nscalar-export\n[]\n" +
		"TERM=y\n"
	if out, st := runZsh(t, t.TempDir(), src); out != want || st != 0 {
		t.Errorf("got %q at %d\nwant %q", out, st, want)
	}
}
