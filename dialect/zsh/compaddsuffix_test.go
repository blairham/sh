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
		// The row is the match alone: measured, `compadd -S ss always auto`
		// lists `always  auto` (#6187).
		{"-S is the whole of it", "", "compadd -S x ff", "ffx", "ff", true},
		{"-S wins over a directory", "", "compadd -W " + w + " -f -S x dd", "ddx", "dd/", true},
		{"a directory gets a slash", "", "compadd -W " + w + " -f dd", "dd/", "dd/", true},
		{"a file gets the space", "", "compadd -W " + w + " -f ff", "ff", "ff", false},
		{"nothing there gets nothing", "", "compadd -W " + w + " -f nosuch", "nosuch", "nosuch", true},
		{"-W is joined with no slash", "", "compadd -W " + tree + " -f dd", "dd", "dd", true},
		{"the test leaves -p out", "", "compadd -W " + w + " -p pre/ -f dd", "pre/dd/", "dd/", true},
		{"an executable is starred", "", "compadd -W " + w + " -f exe", "exe", "exe*", false},
		{"a link is a link", "", "compadd -W " + w + " -f link", "link", "link@", false},
		// No mark, and its column kept: a blank where the slash would be
		// (#6157).
		{"nolisttypes draws no marks", "setopt nolisttypes\n", "compadd -W " + w + " -f dd", "dd/", "dd ", true},
		{"nolisttypes keeps no column for no mark", "setopt nolisttypes\n", "compadd -W " + w + " -f ff", "ff", "ff", false},
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

// TestAListingDrawsTheMatchWithoutWhatIsAroundIt is #6187: a row is the
// match alone, quoted as it is inserted, and never what `compset` moved into
// IPREFIX, nor `-P`, `-p` or `-S` around it. The words are the whole
// replacements, which is what goes on the line. Measured 2026-10-05 through a
// pseudo-terminal against zsh 5.9.2, a `zle -C` widget listing two matches
// after the typed word: each listing is `always  auto`, and `a\ b  a\ d`.
func TestAListingDrawsTheMatchWithoutWhatIsAroundIt(t *testing.T) {
	for _, c := range []struct{ call, line, want string }{
		{"compset -P '*='; compadd always auto", "x --c=a", "--c=always@always --c=auto@auto"},
		{"compadd -p qq always auto", "x qqa", "qqalways@always qqauto@auto"},
		{"compadd -S ss always auto", "x a", "alwaysss@always autoss@auto"},
		{"compset -P '*='; compadd 'a b' 'a d'", "x --c=a", `--c=a\ b@a\ b --c=a\ d@a\ d`},
		// The control: a row that is the word itself is left to the editor.
		{"compadd always auto", "x a", "always@ auto@"},
	} {
		t.Run(c.call, func(t *testing.T) {
			got := drawn(completionCandidatesFor(t, widgetOf(c.call), c.line))
			if got != c.want {
				t.Errorf("%s after %q drew %q, want %q", c.call, c.line, got, c.want)
			}
		})
	}
}
