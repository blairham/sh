// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// A bracket expression written across a separator **is** a bracket here — one
// that can never match, since no metacharacter matches a `/` (#4158).
//
// This column needs no option to show it, which is what makes it the cheapest
// row in the panel: a pattern matching nothing is an error here, so the word
// being a pattern is the difference between a refusal and a line of output.
// bash keeps the word and says nothing, because there it was never a pattern.
//
// Measured 2026-09-23 under `env -i PATH=/usr/bin:/bin LC_ALL=C` from a script
// file on zsh 5.9.2, in a directory holding `keep`:
//
//	print -r -- [a/b]               zsh:1: no matches found: [a/b]
//	setopt nullglob; print -r -- [a/b]   the word is deleted
//
// See interp.Semantics.BracketHoldingASlashIsStillABracket, answered Yes here
// and No in bash.
func TestABracketHoldingASeparatorIsStillAPattern(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			name: "a pattern that matched nothing is refused",
			src:  `print -r -- [a/b]`, want: "zsh:1: no matches found: [a/b]", status: 1,
		},
		{
			name: "and is deleted where the option says to delete one",
			src:  `setopt nullglob; print -r -- "[" [a/b] "]"`, want: "[ ]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runZsh(t, dir, c.src)
			if c.status != 0 {
				if !containsLine(out, c.want) || st != c.status {
					t.Errorf("%q said %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
				}
				return
			}
			if out != c.want || st != c.status {
				t.Errorf("%q said %q (status %d), want %q at %d", c.src, out, st, c.want, c.status)
			}
		})
	}
}
