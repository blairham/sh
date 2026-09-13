// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// `self-insert` is a name this shell has.
//
// It is what a printable key does when nothing else claims it, so it is not a
// key anybody binds — but it is a name a shell can **redefine**, and that is
// the whole of why it has to be in the table. A syntax highlighter wraps every
// name in `$widgets` and recolors the line after each one; the name it most
// needs is the one that runs when a person types.
//
// Measured against zsh 5.9.2 with the same rc: `${+widgets[self-insert]}` was
// 1 there and 0 here, and `${#widgets}` 386 against 43 (#2485).

func TestSelfInsertIsAWidgetThisShellNames(t *testing.T) {
	out, st := runZsh(t, t.TempDir(),
		"print -r -- \"has=${+widgets[self-insert]} is=[${widgets[self-insert]}]\"\n")
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	// `builtin` is what the shell being imitated calls one of its own, and it
	// is what this must say too — a highlighter reads the value to decide how
	// to wrap, and `builtin` is the branch that calls through with a dot.
	if want := "has=1 is=[builtin]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// And the editor is told the name, or a printable key never reaches the shell
// at all.
//
// The two halves are separate and both are needed: the table above is what a
// plugin reads to decide what to wrap, and this is what makes the wrapping
// take effect. A dialect with one and not the other loads a highlighter that
// wraps `self-insert` and still never runs.
func TestTheEditorIsToldWhatTypingIsCalled(t *testing.T) {
	if got := zsh.EditorStyle().SelfInsertWidget; got != "self-insert" {
		t.Errorf("SelfInsertWidget = %q, want self-insert", got)
	}
}

// A widget put in front of typing intercepts a printable key, and reaches the
// real insertion by asking for the editor's own action.
//
// This is the shape every syntax highlighter uses — `zle -N self-insert
// wrapper`, and the wrapper calls `zle .self-insert`. Driven here through the
// dialect's round trip rather than a terminal, which is what repl's own tests
// cover.
func TestAWrapperAroundTypingReachesTheEditor(t *testing.T) {
	r, out := zleRunner(t, "w() { zle .self-insert }\nzle -N self-insert w\n")
	ed := &stubEditor{gives: map[repl.Widget]repl.Line{
		repl.WidgetSelfInsert: {Buffer: "x", Cursor: 1},
	}}
	line, ok, said, _ := runWidgetWatching(t, r, out, "self-insert", repl.Line{}, ed)
	if !ok {
		t.Fatal("the wrapper did not run")
	}
	if said != "" {
		t.Errorf("it said %q, want nothing", said)
	}
	if want := []repl.Widget{repl.WidgetSelfInsert}; len(ed.performed) != 1 || ed.performed[0] != want[0] {
		t.Errorf("performed %v, want %v", ed.performed, want)
	}
	if line.Buffer != "x" {
		t.Errorf("line = %q, want the editor's insertion", line.Buffer)
	}
}

// `bindkey` says the name back, which is the half a listing has to answer for.
func TestTypingIsListedBack(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), "bindkey '^G' self-insert\nbindkey '^G'\n")
	if st != 0 {
		t.Fatalf("status %d, output %q", st, out)
	}
	if !strings.Contains(out, "self-insert") {
		t.Errorf("listing = %q, want the name back", out)
	}
}
