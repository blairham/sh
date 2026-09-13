// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hashable writes an executable into the runner's PATH directory, which
// runBash sets to `dir`, so a bare name there is found by a PATH search and
// hashed by running it.
func hashable(t *testing.T, dir, name string) {
	t.Helper()
	at := filepath.Join(dir, name)
	if err := os.WriteFile(at, []byte("#!/bin/sh\n:\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

// `BASH_CMDS` is the command hash written as an association, and it is a view
// in both directions: a read follows the table as it is now, and an
// assignment hashes a command.
func TestBashCmdsIsTheCommandHash(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"one key",
			`zzc; echo "[${BASH_CMDS[zzc]##*/}]"`,
			"[zzc]\n",
		},
		{
			"the count and the keys",
			`zzc; echo "${#BASH_CMDS[@]} ${!BASH_CMDS[@]}"`,
			"1 zzc\n",
		},
		{
			// A name nothing has run is empty and is not in the roster,
			// which is what makes `${BASH_CMDS[x]:-d}` take the default.
			"a name nothing ran",
			`echo "[${BASH_CMDS[nosuch]}] ${#BASH_CMDS[@]}"`,
			"[] 0\n",
		},
		{
			// A view and not a snapshot: read, run, read again in one shell
			// gives two different answers.
			"a later run is there",
			`echo "${#BASH_CMDS[@]}"; zzc; echo "${#BASH_CMDS[@]}"`,
			"0\n1\n",
		},
		{
			// The write half. An assignment hashes a command exactly as
			// `hash -p` does, at zero hits — measured.
			"an assignment hashes",
			`BASH_CMDS[zzw]=/bin/echo; hash; hash -t zzw`,
			"hits\tcommand\n   0\t/bin/echo\n/bin/echo\n",
		},
		{
			// And an unset of one element does *not* take the entry away,
			// which is bash's answer and not an omission here.
			"an unset element leaves the entry",
			`BASH_CMDS[zzw]=/bin/echo; unset "BASH_CMDS[zzw]"; hash -t zzw; echo "st=$?"`,
			"/bin/echo\nst=0\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hashable(t, dir, "zzc")
			out, st := runBash(t, dir, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status %d, want %q", out, st, tc.want)
			}
		})
	}
}

// A subshell's entry is the subshell's, which comes free from the view being
// a function of the runner rather than a table copied into the name.
func TestBashCmdsFollowsTheSubshell(t *testing.T) {
	dir := t.TempDir()
	hashable(t, dir, "zzc")
	out, _ := runBash(t, dir, `( zzc; echo "in=${#BASH_CMDS[@]}" ); echo "out=${#BASH_CMDS[@]}"`)
	if !strings.Contains(out, "in=1") || !strings.Contains(out, "out=0") {
		t.Errorf("out = %q, want the subshell to have kept its own", out)
	}
}
