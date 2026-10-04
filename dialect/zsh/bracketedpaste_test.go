// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"slices"
	"testing"

	"github.com/blairham/sh/repl"
)

// `bracketed-paste` is a widget a person can call and redefine, and both of
// the spellings a paste plugin uses from inside its own widget reach the
// editor's paste (#5865).
//
// Measured 2026-10-04 against zsh 5.9.2 through a pseudo-terminal, with
// `zle -N bracketed-paste w` and a paste of `a⏎b`:
//
//	w() { zle .bracketed-paste; BUFFER="<$BUFFER>" }    → `<a⏎b>` in the line
//	w() { zle .bracketed-paste P; BUFFER="got:${(q)P}" } → `got:a$'\n'b`, status 0
//
// The second inserts nothing: the paste goes in the parameter and the line is
// the widget's to write. What the editor reads off the terminal is repl's to
// answer and its tests pin it; this pins the naming — that the dotted name
// reaches WidgetBracketedPaste, and that a name after it asks for the text
// rather than the insert.
func TestABracketedPasteWidgetReachesTheEditorsPaste(t *testing.T) {
	t.Run("inserted", func(t *testing.T) {
		r, out := zleRunner(t, "w() { zle .bracketed-paste; print -r -- \"rc=$?\"; }\nzle -N bracketed-paste w\n")
		ed := &stubEditor{gives: map[repl.Widget]repl.Line{
			repl.WidgetBracketedPaste: {Buffer: "a\nb", Cursor: 3},
		}}
		line, ok, printed, _ := runWidgetWatching(t, r, out, "bracketed-paste", repl.Line{}, ed)
		if !ok {
			t.Fatal("the widget did not run")
		}
		if want := []repl.Widget{repl.WidgetBracketedPaste}; !slices.Equal(ed.performed, want) {
			t.Errorf("the editor was asked for %v, want %v", ed.performed, want)
		}
		if printed != "rc=0\n" {
			t.Errorf("the widget printed %q, want %q", printed, "rc=0\n")
		}
		if want := (repl.Line{Buffer: "a\nb", Cursor: 3}); line != want {
			t.Errorf("line back = %+v, want %+v", line, want)
		}
	})
	t.Run("into a parameter", func(t *testing.T) {
		r, out := zleRunner(t, "w() { zle .bracketed-paste P; print -r -- \"rc=$? ${(q)P}\"; }\nzle -N bracketed-paste w\n")
		ed := &stubEditor{paste: "a\nb"}
		line, ok, printed, _ := runWidgetWatching(t, r, out, "bracketed-paste", repl.Line{Buffer: "x", Cursor: 1}, ed)
		if !ok {
			t.Fatal("the widget did not run")
		}
		if want := "rc=0 a$'\\n'b\n"; printed != want {
			t.Errorf("the widget printed %q, want %q", printed, want)
		}
		if len(ed.performed) != 0 {
			t.Errorf("the editor was asked to insert as well: %v", ed.performed)
		}
		if want := (repl.Line{Buffer: "x", Cursor: 1}); line != want {
			t.Errorf("line back = %+v, want %+v — the paste went into the line", line, want)
		}
	})
}

// And the key is listed under its name, the way the standard keymap lists it:
// `bindkey | grep 200` in zsh 5.9.2 is `"^[[200~" bracketed-paste`, and with
// the widget missing this shell listed no such key at all.
func TestTheStandardKeymapListsThePasteMarker(t *testing.T) {
	_, out := zleRunner(t, "bindkey '^[[200~'\n")
	if want := "\"^[[200~\" bracketed-paste\n"; out.String() != want {
		t.Errorf("bindkey '^[[200~' = %q, want %q", out.String(), want)
	}
}
