// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// TestTheSpecialWidgetsAreCalledWhereZshCallsThem: `zle-line-init` once the
// prompt is drawn, `zle-line-pre-redraw` before each redraw and once more as
// the line is accepted, then `zle-line-finish` — and a finish widget's change
// to the line is what is accepted. Measured 2026-10-02 through a
// pseudo-terminal against zsh 5.9.2; see specialwidgets.go (#5398).
func TestTheSpecialWidgetsAreCalledWhereZshCallsThem(t *testing.T) {
	var out strings.Builder
	var calls []string
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("a\n"), &out
	e.specials = true
	e.runFunc = func(name string, in Line, _ Actions) (Line, bool) {
		if !strings.HasPrefix(name, "zle-") {
			return in, false
		}
		calls = append(calls, name+"["+in.Buffer+"]")
		if name == "zle-line-finish" {
			return Line{Buffer: "changed", Cursor: 7}, true
		}
		return in, true
	}
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	want := "zle-line-init[] zle-line-pre-redraw[a] zle-line-pre-redraw[a] zle-line-finish[a]"
	if got := strings.Join(calls, " "); got != want {
		t.Errorf("calls = %s, want %s", got, want)
	}
	if line != "changed" {
		t.Errorf("line = %q, want what the finish widget left", line)
	}
}

// And a session whose shell defines none of them reads exactly as before:
// the names are asked for and declined, and nothing else changes.
func TestNoSpecialWidgetsLeavesTheLineAlone(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("abc\n"), &out
	e.specials = true
	e.runFunc = func(string, Line, Actions) (Line, bool) { return Line{}, false }
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "abc" {
		t.Errorf("line = %q, %v, want abc", line, err)
	}
}

// A pre-redraw widget that redraws — `zle -R` inside one — asks for no second
// pre-redraw from inside itself, which would never end.
func TestAPreRedrawThatRedrawsDoesNotRecur(t *testing.T) {
	var out strings.Builder
	calls := 0
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("a\n"), &out
	e.specials = true
	e.runFunc = func(name string, in Line, ed Actions) (Line, bool) {
		if name != "zle-line-pre-redraw" {
			return in, false
		}
		calls++
		if calls > 10 {
			t.Fatal("the pre-redraw widget recurred")
		}
		ed.Redisplay(in)
		return in, true
	}
	if line, err := e.readLine(drawPrompt("$ ")); err != nil || line != "a" {
		t.Errorf("line = %q, %v, want a", line, err)
	}
	if calls != 2 {
		t.Errorf("pre-redraw ran %d times, want 2 — the key's redraw and the accept's", calls)
	}
}
