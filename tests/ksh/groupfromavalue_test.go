// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// groupDir is the directory every row below is measured in. The third file is
// the one that makes the claim falsifiable: without a file literally named
// `ice|other.zsh`, "the group is text" and "the group is a group whose one
// branch holds a literal bar" both leave the word standing unchanged, and the
// probe set #1499 was filed with could not tell them apart.
func groupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"ice.zsh", "other.zsh", "ice|other.zsh"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// `(`, `)` and `|` arriving out of an expansion are literal characters in this
// shell's pathname expansion; every other pattern metacharacter is live. The
// group's shape is the source's and its leaves are the value's.
//
// Measured 2026-09-12 on ksh93u+ 2012-08-01 in exactly the directory above.
func TestAGroupsSyntaxCannotComeFromAValue(t *testing.T) {
	dir := groupDir(t)
	for _, tc := range []struct{ name, src, want string }{
		// The row that matched a file, which is what says the group is a
		// group and its bar is a character rather than a choice.
		{"a bar from a value is a character", `L="ice|other"; echo @($L).zsh`, "ice|other.zsh\n"},
		{"and the bar alone from a value", `B='|'; echo @(ice${B}other).zsh`, "ice|other.zsh\n"},
		// A bar written in the source still separates branches, with the
		// branch beside it coming from a value.
		{"a bar in the source still branches", `L=ice; echo @($L|other).zsh`, "ice.zsh other.zsh\n"},
		// Every other metacharacter out of a value is live, inside a group
		// as much as outside one.
		{"a star from a value globs in a group", `L="ice*"; echo @($L).zsh`, "ice.zsh ice|other.zsh\n"},
		{"and a question mark", `L="ic?"; echo @($L).zsh`, "ice.zsh\n"},
		{"and a bracket outside one", `S="[io]*"; echo $S`, "ice.zsh ice|other.zsh other.zsh\n"},
		// A whole group out of a value is dead in all three characters, so
		// it matches nothing and the word stands.
		{"a whole group from a value is text", `L="@(ice|other)"; echo $L.zsh`, "@(ice|other).zsh\n"},
		// The condition surface does not do it, which is what keeps this an
		// axis about pathname expansion rather than about the matcher.
		{"a condition reads the bar", `L="a|b"; [[ a == @($L) ]]; echo "m=$?"`, "m=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runKsh(t, dir, "cd "+dir+"\n"+tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q at %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
