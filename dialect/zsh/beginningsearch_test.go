// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/repl"
)

// The five widgets `up-line-or-beginning-search` and the `*-match` family are
// built from (#5910): each name reaches the editor action repl calls by its own
// name, and what that action answers is the widget's status. What the actions
// do is repl's to pin — see repl/beginningsearch_test.go.
func TestTheBeginningSearchNamesReachTheEditor(t *testing.T) {
	for name, want := range map[string]repl.Widget{
		"history-beginning-search-backward": repl.WidgetHistoryBeginningSearchBackward,
		"history-beginning-search-forward":  repl.WidgetHistoryBeginningSearchForward,
		"up-line":                           repl.WidgetUpLine,
		"down-line":                         repl.WidgetDownLine,
	} {
		t.Run(name, func(t *testing.T) {
			r, out := zleRunner(t, "w() { zle "+name+"; print -r -- st=$?; zle ."+name+"; print -r -- st=$? }\nzle -N w\n")
			ed := &stubEditor{gives: map[repl.Widget]repl.Line{want: {Buffer: "x", Cursor: 1, Status: 1}}}
			_, ok, said, _ := runWidgetWatching(t, r, out, "w", repl.Line{}, ed)
			if !ok {
				t.Fatal("the widget did not run")
			}
			if len(ed.performed) != 2 || ed.performed[0] != want || ed.performed[1] != want {
				t.Errorf("performed %v, want %v twice", ed.performed, want)
			}
			if said != "st=1\nst=1\n" {
				t.Errorf("said %q, want the action's status both times", said)
			}
		})
	}
}

// `zle copy-region-as-kill STRING` hands the string to the editor's kill and
// leaves the line alone; without a string it is refused by name, since this
// editor has no mark to copy from.
func TestCopyRegionAsKillHandsTheStringToTheEditor(t *testing.T) {
	r, out := zleRunner(t, `w() { zle copy-region-as-kill "foo bar" extra; print -r -- st=$? "[$BUFFER|$CURSOR]"; zle copy-region-as-kill; print -r -- st=$? }
zle -N w
`)
	ed := &stubEditor{}
	_, ok, said, _ := runWidgetWatching(t, r, out, "w", repl.Line{Buffer: "xy", Cursor: 1}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	if ed.killed != "foo bar" {
		t.Errorf("killed %q, want %q", ed.killed, "foo bar")
	}
	const want = "st=0 [xy|1]\nw:zle: copy-region-as-kill without a string is not implemented yet\nst=1\n"
	if said != want {
		t.Errorf("said %q, want %q", said, want)
	}
}

// They are the editor's own, so `zle -la` names them.
func TestTheBeginningSearchNamesAreListed(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), `for w in history-beginning-search-backward history-beginning-search-forward up-line down-line copy-region-as-kill; do
  zle -la $w && print -r -- $w
done
`)
	const want = "history-beginning-search-backward\nhistory-beginning-search-forward\nup-line\ndown-line\ncopy-region-as-kill\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
