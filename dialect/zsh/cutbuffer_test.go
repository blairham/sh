// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// `$CUTBUFFER` inside a widget is the kill the editor handed in, and what the
// widget assigns is the kill that goes back (#5916). Measured 2026-10-04
// through a pseudo-terminal against zsh 5.9.2: `${(t)CUTBUFFER}` is
// `scalar-local-special`, `CUTBUFFER=hello` then `zle yank` inserts `hello`,
// and the next widget reads `hello` back.
func TestCutBufferIsTheKillTheLineCarries(t *testing.T) {
	r, out := zleRunner(t, `w() { print -r -- "${(t)CUTBUFFER} [$CUTBUFFER]"; CUTBUFFER=hello }
zle -N w
`)
	ed := &stubEditor{cut: "earlier"}
	_, ok, said, _ := runWidgetWatching(t, r, out, "w", repl.Line{Buffer: "ab", Cursor: 2}, ed)
	if !ok {
		t.Fatal("the widget did not run")
	}
	if said != "scalar-local-special [earlier]\n" {
		t.Errorf("said %q, want the editor's kill, as a local special", said)
	}
	if ed.cut != "hello" {
		t.Errorf("the editor's kill is %q, want %q", ed.cut, "hello")
	}
}

// What an action the widget calls leaves in the kill is what `$CUTBUFFER`
// reads straight after, which is what lets a widget join a kill of its own
// onto the one before it.
func TestCutBufferFollowsAKillTheEditorMade(t *testing.T) {
	r, out := zleRunner(t, `w() { zle backward-kill-word; print -r -- "[$CUTBUFFER]" }
zle -N w
`)
	ed := &killingEditor{stubEditor: stubEditor{gives: map[repl.Widget]repl.Line{
		repl.WidgetKillWordBefore: {Buffer: "one ", Cursor: 4},
	}}, kills: map[repl.Widget]string{repl.WidgetKillWordBefore: "two"}}
	out.Reset()
	if _, ok := zsh.RunWidget(r, repl.WithActions(context.Background(), ed), "w", repl.Line{Buffer: "one two", Cursor: 7}); !ok {
		t.Fatal("the widget did not run")
	}
	if said := out.String(); said != "[two]\n" {
		t.Errorf("said %q, want the kill the action made", said)
	}
}

// killingEditor is the stub with actions that kill: what each leaves in the
// kill, as the editor's own kills do.
type killingEditor struct {
	stubEditor
	kills map[repl.Widget]string
}

func (e *killingEditor) Perform(w repl.Widget, in repl.Line) (repl.Line, bool) {
	if text, kills := e.kills[w]; kills {
		e.cut = text
	}
	return e.stubEditor.Perform(w, in)
}

// Outside a widget there is no such parameter, measured: `${+CUTBUFFER}` is 0.
func TestCutBufferIsNotThereOutsideAWidget(t *testing.T) {
	out, _ := runZsh(t, t.TempDir(), "print -r -- ${+CUTBUFFER}\n")
	if out != "0\n" {
		t.Errorf("got %q, want 0", out)
	}
}
