// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// TestBindkeyMakesAndNamesKeymaps is `bindkey -N` and `bindkey -A`, each row
// measured 2026-10-05 on zsh 5.9.2 under `zsh -f` (#5969). See keymaps.go.
func TestBindkeyMakesAndNamesKeymaps(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"bindkey -N mymap emacs; echo st=$?; bindkey -l mymap; bindkey -M mymap '^A'", "st=0\nmymap\n\"^A\" beginning-of-line\n"},
		{"bindkey -N mymap emacs; bindkey -M mymap '^Xq' undo; bindkey -M emacs '^Xq'", "\"^Xq\" undefined-key\n"},
		{"bindkey -N e2; echo st=$?; bindkey -M e2 '^A'; bindkey -M e2; bindkey -lL e2", "st=0\n\"^A\" undefined-key\nbindkey -N e2\n"},
		{
			"bindkey -A emacs foo; echo st=$?; bindkey -M foo '^Xq' undo; bindkey -M emacs '^Xq'; bindkey -lL foo emacs main",
			"st=0\n\"^Xq\" undo\nbindkey -A emacs foo\nbindkey -N emacs\nbindkey -A emacs main\n",
		},
		{"bindkey -N foo emacs; bindkey -M foo '^Xq' undo; bindkey -N foo; echo st=$?; bindkey -M foo '^Xq'", "st=0\n\"^Xq\" undefined-key\n"},
		{
			"bindkey -N mymap emacs; bindkey -A mymap main; echo st=$?; bindkey -lL main; bindkey '^Xz' undo; bindkey -M mymap '^Xz'; bindkey -M emacs '^Xz'",
			"st=0\nbindkey -A mymap main\n\"^Xz\" undo\n\"^Xz\" undefined-key\n",
		},
		{
			"bindkey -N mymap emacs; bindkey -A mymap main; bindkey -A emacs mymap; bindkey -lL main mymap; bindkey -e; bindkey -lL main",
			"bindkey -A mymap main\nbindkey -A emacs mymap\nbindkey -A emacs main\n",
		},
		{"bindkey -N a emacs; bindkey -A emacs b; print -r -- ${(o)keymaps}", ".safe a b command emacs isearch main vicmd viins viopp visual\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want || st != 0 {
			t.Errorf("%s\nprinted %q at %d, want %q", c.src, out, st, c.want)
		}
	}
}

// TestBindkeyNewAndLinkRefusals: each status 1, measured on zsh 5.9.2.
func TestBindkeyNewAndLinkRefusals(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{"bindkey -N foo nosuch", "zsh:bindkey:1: no such keymap `nosuch'\n"},
		{"bindkey -A nosuch foo", "zsh:bindkey:1: no such keymap `nosuch'\n"},
		{"bindkey -N", "zsh:bindkey:1: not enough arguments for -N\n"},
		{"bindkey -N a b c", "zsh:bindkey:1: too many arguments for -N\n"},
		{"bindkey -A x", "zsh:bindkey:1: not enough arguments for -A\n"},
		{"bindkey -A emacs a b", "zsh:bindkey:1: too many arguments for -A\n"},
		{"bindkey -N .safe", "zsh:bindkey:1: keymap name `.safe' is protected\n"},
		{"bindkey -A emacs .safe", "zsh:bindkey:1: keymap name `.safe' is protected\n"},
	} {
		if out, st := runZsh(t, t.TempDir(), c.src+"\n"); out != c.want || st != 1 {
			t.Errorf("%s printed %q at %d, want %q at 1", c.src, out, st, c.want)
		}
	}
}

// TestAMadeKeymapSelectedAsMainDrivesTheEditor: a copy of viins selected as
// main is vi editing, and a key bound into it reaches the editor, where a key
// bound only in emacs does not.
func TestAMadeKeymapSelectedAsMainDrivesTheEditor(t *testing.T) {
	r := bindkeyRunner(t, "bindkey -N myvi viins\nbindkey -A myvi main\nbindkey -M myvi '^Xw' undo\nbindkey -M emacs '^Xe' undo\n")
	if !zsh.ViEditing(r) {
		t.Error("a copy of viins selected as main is not vi editing")
	}
	got := zsh.KeyBindings(r, repl.KeymapMain)
	if b, ok := got["\x18w"]; !ok || b.Widget != repl.WidgetUndo {
		t.Errorf("^Xw in the made keymap = %+v (present %v), want undo", b, ok)
	}
	if _, ok := got["\x18e"]; ok {
		t.Error("^Xe, bound only in emacs, reached the editor")
	}
	r = bindkeyRunner(t, "bindkey -N mine emacs\nbindkey -A mine main\n")
	if zsh.ViEditing(r) {
		t.Error("a copy of emacs selected as main is vi editing")
	}
}
