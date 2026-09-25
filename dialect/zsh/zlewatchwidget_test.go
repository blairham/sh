// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/repl"
	"github.com/blairham/sh/syntax"
)

// `zle some-widget` from inside a plain `zle -F` handler, which is how every
// asynchronous suggestion in this shell reaches the screen.
//
// The handler itself is an ordinary function and has no line — that is
// TestAPlainHandlerIsCalledWithTheDescriptorAndLeavesTheLineAlone, and it
// still holds. What this file is about is the *other* half of the same
// measurement, which nothing asked until #4413: a handler may call `zle`, and
// a widget has the line whoever called it.
//
// zsh-autosuggestions is the plugin the pair was measured against. Its
// response handler is a plain handler — `_zsh_autosuggest_async_response`,
// armed with `zle -F "$fd"` and no `-w` — and its whole body is a read
// followed by `zle autosuggest-suggest -- "$suggestion"`, a widget whose one
// job is to set `POSTDISPLAY`. So a shell where a widget called from a
// handler cannot reach the line is a shell where that plugin's async path —
// the path its own README recommends and the path a plugin manager configures
// — draws nothing at all, while its synchronous path, which reaches the line
// through an ordinary keystroke's widget, is right.

func TestAWidgetCalledFromAPlainHandlerIsGivenTheLine(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("lo-world\n"); err != nil {
		t.Fatal(err)
	}
	// Written the way the plugin writes it: the handler reads the descriptor
	// and hands what it read to a widget, and the widget is the only thing
	// that touches the line.
	src := "sugg(){ POSTDISPLAY=$SUG; BUFFER=$BUFFER; }\nzle -N sugg\n" +
		"h(){ IFS= read -u $1 -r SUG; zle sugg; }\n" +
		"zle -F " + strconv.Itoa(shellFd) + " h"
	r, out := watchRunnerWith(t, src, func(rr *interp.Runner) { rr.InheritedFiles = inherited })

	in := repl.Line{Buffer: "echo hel", Cursor: 8}
	got, changed := zsh.DescriptorReady(r, context.Background(), systemFd, in)
	if out.String() != "" {
		t.Fatalf("the handler complained: %q", out.String())
	}
	if !changed {
		t.Fatal("the editor was told there is nothing to draw, so the suggestion never reaches the screen")
	}
	if got.Postdisplay != "lo-world" {
		t.Errorf("postdisplay = %q, want %q — the widget's POSTDISPLAY did not come back", got.Postdisplay, "lo-world")
	}
	// And the widget was handed the line it was editing rather than an empty
	// one, which is the half that would make a wrapper overwrite the line.
	if got.Buffer != "echo hel" || got.Cursor != 8 {
		t.Errorf("line = %#v, want the buffer and cursor it went in with", got)
	}
}

// TestAHandlerMayCallAWidgetTwice: the first call must not end the editor for
// the second. runWidgetFunction clears the state it opened on the way out,
// and a handler is not over when one widget is.
func TestAHandlerMayCallAWidgetTwice(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	src := "one(){ POSTDISPLAY=first }\nzle -N one\n" +
		"two(){ POSTDISPLAY=$POSTDISPLAY-second }\nzle -N two\n" +
		"h(){ IFS= read -u $1 -r _; zle one; zle two; }\n" +
		"zle -F " + strconv.Itoa(shellFd) + " h"
	r, out := watchRunnerWith(t, src, func(rr *interp.Runner) { rr.InheritedFiles = inherited })

	got, changed := zsh.DescriptorReady(r, context.Background(), systemFd, repl.Line{Buffer: "b", Cursor: 1})
	if out.String() != "" {
		t.Fatalf("the handler complained: %q", out.String())
	}
	if !changed {
		t.Fatal("nothing came back")
	}
	// The second widget read what the first left, which is what says both ran
	// and that they shared one line rather than each getting a fresh one.
	if got.Postdisplay != "first-second" {
		t.Errorf("postdisplay = %q, want %q", got.Postdisplay, "first-second")
	}
}

// TestAHandlerThatRunsNoWidgetStillDrawsNothing is the measured behavior this
// must not have cost: a plain handler prints where the cursor was and the
// editor draws nothing afterwards. It is the control for the two above — a
// change that simply returned `true` from every handler would pass both of
// them and fail this.
func TestAHandlerThatRunsNoWidgetStillDrawsNothing(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	src := "h(){ IFS= read -u $1 -r _; }\nzle -F " + strconv.Itoa(shellFd) + " h"
	r, _ := watchRunnerWith(t, src, func(rr *interp.Runner) { rr.InheritedFiles = inherited })

	in := repl.Line{Buffer: "half typed", Cursor: 4}
	got, changed := zsh.DescriptorReady(r, context.Background(), systemFd, in)
	if changed {
		t.Error("a handler that ran no widget asked for a redraw")
	}
	if got != in {
		t.Errorf("line = %#v, want it untouched as %#v", got, in)
	}
}

// TestAWidgetCalledFromAHandlerLeavesNothingBehind: the line a handler was
// holding is the editor's, not the session's, and a script at the next prompt
// must find the parameters unset exactly as it did before.
func TestAWidgetCalledFromAHandlerLeavesNothingBehind(t *testing.T) {
	shellFd, systemFd, write, inherited := pipeAt(t)
	if _, err := write.WriteString("x\n"); err != nil {
		t.Fatal(err)
	}
	src := "sugg(){ POSTDISPLAY=pd }\nzle -N sugg\n" +
		"h(){ IFS= read -u $1 -r _; zle sugg; }\n" +
		"zle -F " + strconv.Itoa(shellFd) + " h"
	r, out := watchRunnerWith(t, src, func(rr *interp.Runner) { rr.InheritedFiles = inherited })
	zsh.DescriptorReady(r, context.Background(), systemFd, repl.Line{Buffer: "echo hel", Cursor: 8})
	out.Reset()

	runOneMore(t, r, `print -r -- "after buffer=[${BUFFER-UNSET}] post=[${POSTDISPLAY-UNSET}] widget=[${WIDGET-UNSET}]"`)
	want := "after buffer=[UNSET] post=[UNSET] widget=[UNSET]\n"
	if out.String() != want {
		t.Errorf("at the next prompt = %q, want %q", out.String(), want)
	}
}

// runOneMore runs one more line on a runner that has already been used, which is
// what "at the next prompt" means here.
func runOneMore(t *testing.T, r *interp.Runner, src string) {
	t.Helper()
	f, err := syntax.Parse(src, zsh.Dialect())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
}

// TestTheDoubleDashEndingAWidgetsOptionsIsNotAnArgument pins the table in
// widgetCallArgs, which is the difference between an async suggestion reading
// `lo-world` and reading `--`.
//
// It is measured against zsh 5.9.2 through a pseudo-terminal — the rows are
// in that function's own comment — and it is asserted here through the widget
// table rather than through a terminal, because what is being checked is the
// argument list a widget is called with and nothing about the screen.
func TestTheDoubleDashEndingAWidgetsOptionsIsNotAnArgument(t *testing.T) {
	for _, row := range []struct {
		call string
		want string
	}{
		// One `--` immediately after the name is the marker and goes.
		{`zle inner -- "a b"`, "[1]a b"},
		{`zle inner -- -x`, "[1]-x"},
		{`zle inner --`, "[0]"},
		// The second of a pair is an ordinary operand, and so is one that
		// follows an operand: it ends an option list rather than meaning
		// anything by itself.
		{`zle inner -- -- q`, "[2]--|q"},
		{`zle inner a -- b`, "[3]a|--|b"},
		{`zle inner a b`, "[2]a|b"},
	} {
		src := `inner() { print -r -- "[$#]${(j:|:)@}" }` + "\nzle -N inner\n" +
			"outer() { " + row.call + " }\nzle -N outer\n"
		r, out := watchRunner(t, src)
		if _, ok := zsh.RunWidget(r, context.Background(), "outer", repl.Line{}); !ok {
			t.Fatalf("%s: the outer widget did not run", row.call)
		}
		if got := strings.TrimRight(out.String(), "\n"); got != row.want {
			t.Errorf("%s gave the widget %s, want %s", row.call, got, row.want)
		}
	}
}
