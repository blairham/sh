// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"
)

// How a lone match is finished and how a file is drawn (#6145). Measured
// 2026-10-05 through a pseudo-terminal against zsh 5.9.2, a completion widget
// adding one match to `x ` and `Z` typed after it; Open is "nothing after
// it", the word is what goes in, and the row is what a listing draws.
func TestCompaddSuffixesAndFileMarks(t *testing.T) {
	tree := t.TempDir()
	for _, d := range []string{"dd"} {
		if err := os.Mkdir(filepath.Join(tree, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tree, "ff"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tree, "exe"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("ff", filepath.Join(tree, "link")); err != nil {
		t.Fatal(err)
	}
	w := tree + "/"
	for _, c := range []struct {
		name, setup, call string
		word, row         string
		open              bool
	}{
		{"no suffix is a space", "", "compadd ff", "ff", "", false},
		{"an empty -S is nothing", "", "compadd -S '' ff", "ff", "", true},
		{"-S is the whole of it", "", "compadd -S x ff", "ffx", "", true},
		{"-S wins over a directory", "", "compadd -W " + w + " -f -S x dd", "ddx", "dd/", true},
		{"a directory gets a slash", "", "compadd -W " + w + " -f dd", "dd/", "dd/", true},
		{"a file gets the space", "", "compadd -W " + w + " -f ff", "ff", "ff", false},
		{"nothing there gets nothing", "", "compadd -W " + w + " -f nosuch", "nosuch", "nosuch", true},
		{"-W is joined with no slash", "", "compadd -W " + tree + " -f dd", "dd", "dd", true},
		{"the test leaves -p out", "", "compadd -W " + w + " -p pre/ -f dd", "pre/dd/", "dd/", true},
		{"an executable is starred", "", "compadd -W " + w + " -f exe", "exe", "exe*", false},
		{"a link is a link", "", "compadd -W " + w + " -f link", "link", "link@", false},
		{"nolisttypes draws no marks", "setopt nolisttypes\n", "compadd -W " + w + " -f dd", "dd/", "dd", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionCandidatesFor(t, c.setup+widgetOf(c.call), "x ")
			if len(got) != 1 {
				t.Fatalf("got %d candidates, want 1: %+v", len(got), got)
			}
			if got[0].Word != c.word || got[0].Display != c.row || got[0].Open != c.open {
				t.Errorf("word %q row %q open %v, want %q %q %v",
					got[0].Word, got[0].Display, got[0].Open, c.word, c.row, c.open)
			}
		})
	}
}
