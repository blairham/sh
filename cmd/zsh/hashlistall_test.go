// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestHashListAllDecidesWhetherCommandsFillsTheTable is #5159's V06parameter
// front `$commands look-up with no_hash_list_all`. `hashlistall` was recorded
// and did nothing, so `$commands` always filled the table from PATH. See
// dialect/zsh's fillCommandHashWhereListed for the grid.
//
// Every row measured 2026-10-02 on zsh 5.9.2 (`/opt/homebrew/bin/zsh -f`),
// from a script file in a directory of its own.
func TestHashListAllDecidesWhetherCommandsFillsTheTable(t *testing.T) {
	const body = `
: > tool1; /bin/chmod +x tool1
path=( $PWD )
rehash
print -r -- "rehash=${#commands}"
a=$commands[tool1]
/bin/rm tool1
b=$commands[tool1]
print -r -- "a=[${a:t}] b=[${b:t}] +=$+commands[tool1] n=${#commands}"
`
	for _, c := range []struct{ opts, want string }{
		{"hashlistall hashcmds", "rehash=2\na=[tool1] b=[tool1] +=1 n=2\n"},
		{"hashlistall nohashcmds", "rehash=2\na=[tool1] b=[tool1] +=1 n=2\n"},
		{"nohashlistall hashcmds", "rehash=0\na=[tool1] b=[tool1] +=1 n=1\n"},
		{"nohashlistall nohashcmds", "rehash=0\na=[tool1] b=[] +=0 n=0\n"},
	} {
		t.Run(c.opts, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			if err := os.WriteFile(filepath.Join(dir, "s.zsh"), []byte("setopt "+c.opts+"\n"+body), 0o600); err != nil {
				t.Fatal(err)
			}
			out, errs, _ := runZsh(t, "-f", "./s.zsh")
			if out != c.want || errs != "" {
				t.Errorf("got %q %q, want %q", out, errs, c.want)
			}
		})
	}
}
