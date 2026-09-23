// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// `POSTDISPLAY` is the text a widget asks to have drawn after the line without
// it being part of the line — what an inline suggestion is made of.
//
// It is the half of zsh-autosuggestions that survives #4211: with the queue
// parameters answered the widget runs to the end, asks the history strategy for
// a suggestion, gets one, and assigns it to a name that did not exist — so the
// assignment succeeded, an ordinary variable was created, and nothing was ever
// drawn. No error to notice and the plugin's own machinery all reporting
// success, which is the shape that hides (#4217).
//
// Measured 2026-09-22 through a pseudo-terminal against zsh 5.9.2, a widget
// bound to a key. What is asserted here is the parameter; what it *draws* is
// repl's, and repl/postdisplaypty_test.go drives a terminal for it.
func TestAWidgetSetsTheTextDrawnAfterTheLine(t *testing.T) {
	r, out := zleRunner(t, `
		sugg() { POSTDISPLAY=" --sugg"; }
		zle -N sugg
	`)
	line, ok, said := runWidget(t, r, out, "sugg", repl.Line{Buffer: "echo hi", Cursor: 7})
	if !ok {
		t.Fatal("the widget did not run")
	}
	if said != "" {
		t.Errorf("the widget said %q, want nothing", said)
	}
	if want := " --sugg"; line.Postdisplay != want {
		t.Errorf("the editor was handed %q, want %q", line.Postdisplay, want)
	}
	// And the line is the line: a postdisplay is not in it and does not move
	// the cursor. Measured — `$#BUFFER` is 7 and `CURSOR` 7 with one set.
	if line.Buffer != "echo hi" || line.Cursor != 7 {
		t.Errorf("the line came back as %q at %d, want %q at 7", line.Buffer, line.Cursor, "echo hi")
	}
}

// What is already drawn is what a widget reads, which is the half a plugin's
// wrapper depends on: it saves the suggestion, clears it while a new one is
// fetched, and puts one of the two back.
func TestAWidgetReadsTheTextAlreadyDrawnAfterTheLine(t *testing.T) {
	r, out := zleRunner(t, `
		seen() { print -r -- "[$POSTDISPLAY][${(t)POSTDISPLAY}][${+POSTDISPLAY}]"; POSTDISPLAY=""; }
		zle -N seen
	`)
	line, ok, said := runWidget(t, r, out,
		"seen", repl.Line{Buffer: "echo hi", Cursor: 7, Postdisplay: " --old"})
	if !ok {
		t.Fatal("the widget did not run")
	}
	// The type word is measured rather than assumed: `scalar-local-special`
	// both before a widget assigns to it and after, which is BUFFER's word and
	// not the read-only one WIDGET carries. A plugin that tests for `local`
	// before trusting a parameter finds it.
	if want := "[ --old][scalar-local-special][1]\n"; said != want {
		t.Errorf("the widget saw %q, want %q", said, want)
	}
	// And a widget may clear it, which is what the wrapper does first.
	if line.Postdisplay != "" {
		t.Errorf("the editor was handed %q, want it cleared", line.Postdisplay)
	}
}

// Outside a widget the name does not exist, which is the rule every line
// parameter here follows: a script that is not editing a line has no line.
func TestTheTextDrawnAfterTheLineIsAWidgetsAlone(t *testing.T) {
	r, out := zleRunner(t, `
		sugg() { POSTDISPLAY=" --sugg"; }
		zle -N sugg
	`)
	if _, ok, _ := runWidget(t, r, out, "sugg", repl.Line{Buffer: "x"}); !ok {
		t.Fatal("the widget did not run")
	}
	out.Reset()
	runZleScript(t, r, `print -r -- "[${+POSTDISPLAY}][$POSTDISPLAY]"`)
	if want := "[0][]\n"; !strings.Contains(out.String(), want) {
		t.Errorf("a script after the widget saw %q, want %q — the parameter is the call's", out.String(), want)
	}
}

// runZleScript runs a line of script through a runner a widget has already run
// on, which is how the "outside a widget" half is asked: the parameter has to
// be gone *after* a call and not merely absent before one.
func runZleScript(t *testing.T, r *interp.Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
}
