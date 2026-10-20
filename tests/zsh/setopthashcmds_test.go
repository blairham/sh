// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// HASH_CMDS is on before a script's first line, and every surface that names
// it has to say so. Measured on zsh 5.9.2 (aarch64-apple-darwin25.4.0),
// 2026-09-25, in `zsh -f -c`:
//
//	[[ -o hashcmds ]]        0
//	${options[hashcmds]}     on
//	setopt                   does not name it
//	unsetopt                 nohashcmds
//	set -o                   nohashcmds            off
//
// The last row is the spelling rule and not a second state: a listing writes
// each option in the direction that is off by default, so an option that is on
// by default is written `no…` and reported `off`. This shell wrote `hashcmds
// off` — the same word for the opposite state — which is #4533.
func TestHashCmdsIsOnBeforeAnythingMovesIt(t *testing.T) {
	dir := t.TempDir()
	// No external command is reachable here — the helper puts the temporary
	// directory alone on PATH — so the rows are picked out with the shell's
	// own `case` rather than with `grep`.
	out, st := runZsh(t, dir, `
row() { while IFS= read -r l; do case $l in *hashcmds*) echo "$1 [$l]";; esac; done; }
[[ -o hashcmds ]] && echo 'cond on' || echo 'cond off'
[[ -o hashall ]] && echo 'hashall on' || echo 'hashall off'
zmodload zsh/parameter
echo "param [$options[hashcmds]] [$options[hashall]] [$options[trackall]]"
setopt   | row setopt
unsetopt | row unsetopt
set -o   | row seto
`)
	if st != 0 {
		t.Fatalf("status %d: %s", st, out)
	}
	wantWholeLines(t, out,
		"cond on",
		"hashall on",
		"param [on] [on] [on]",
		"unsetopt [nohashcmds]",
		"seto [nohashcmds            off]",
	)
	// And a bare `setopt` does not name it at all, which is the half a
	// row-by-row check of the two listings above cannot see: an option at its
	// mode's default belongs to exactly one of them.
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "setopt [") {
			t.Errorf("a bare `setopt` names hashcmds as %q, want it only in `unsetopt`", l)
		}
	}
}

// **The report is the noun here, not the hashing** — and this is the pair that
// says so, because the two questions had agreed on every row anybody had run.
//
// Both shells fill the command hash from a command they ran, in every state of
// the option, so a probe that ran `zzc` and read `hash` back reported the same
// thing under a default of on and a default of off. Holding that fixed and
// reading the *report* is what parts them: the three rows below put an
// identical hashing result beside three different reports, and only the report
// moved when #4533 was fixed.
//
// The third row is what keeps this from being a test that the option is inert:
// with it off, nothing is hashed. So the option is wired to something real,
// and the default it started from was the only thing wrong.
func TestTheHashCmdsReportMovesAndTheHashingDoesNot(t *testing.T) {
	for _, c := range []struct {
		name, prefix, wantReport, wantHash string
	}{
		{"untouched", "", "on", "found"},
		{"set on", "setopt hashcmds\n", "on", "found"},
		{"set off", "unsetopt hashcmds\n", "off", "empty"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "zzhashc")
			if err := os.WriteFile(bin, []byte("#!/bin/sh\n:\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			out, st := runZsh(t, dir, c.prefix+fmt.Sprintf(`
[[ -o hashcmds ]] && echo 'report on' || echo 'report off'
zzhashc
h=$(hash)
case $h in *zzhashc=*) echo 'hash found';; *) echo 'hash empty';; esac
_=%q
`, c.name))
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			wantWholeLines(t, out, "report "+c.wantReport, "hash "+c.wantHash)
		})
	}
}

// The reset keeps its two halves apart, which is the second pair the default
// had to land on. `hashcmds` is in the strict-reset set and not in the 81 a
// bare emulation puts back, so a script that turned it off finds it **on**
// after `emulate -R zsh` and still **off** after a bare `emulate zsh`.
//
// Measured on zsh 5.9.2, 2026-09-25. Without the corrected default the first
// row would answer off as well, since the reset writes `def` — so this is the
// case where the table entry and the backing state have to agree rather than
// merely both be readable.
func TestTheStrictResetPutsHashCmdsBackAndTheBareOneDoesNot(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"strict", "unsetopt hashcmds\nemulate -R zsh\n", "on"},
		{"bare", "unsetopt hashcmds\nemulate zsh\n", "off"},
		// The control: a shell nobody moved reads on under both forms, so a
		// reset stuck at "on" could not pass the bare row above.
		{"strict, unmoved", "emulate -R zsh\n", "on"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), c.src+"[[ -o hashcmds ]] && echo on || echo off\n")
			if st != 0 {
				t.Fatalf("status %d: %s", st, out)
			}
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("after %q the option reads %q, want %q", c.src, got, c.want)
			}
		})
	}
}
