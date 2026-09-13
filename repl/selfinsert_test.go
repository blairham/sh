// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Typing, when the shell has put something in front of it.
//
// Every other action here is something a key is *bound* to. This is what the
// key loop does when nothing else claims the key — so it was not a Widget at
// all, and a shell could not get in front of it. A syntax highlighter needs
// exactly that: it wraps every widget the shell names and recolours the line
// after each one, and the one that matters most is the one that runs when a
// person types (#2485).

// typedWithSelfInsert runs keys through an editor whose shell claims a
// `self-insert` action, with run standing in for the dialect.
func typedWithSelfInsert(t *testing.T, run func(Line, Actions) (Line, bool), keys string) string {
	t.Helper()
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.selfInsert = "self-insert"
	if run != nil {
		e.runFunc = func(name string, in Line, ed Actions) (Line, bool) {
			if name != "self-insert" {
				t.Errorf("asked for %q, want self-insert", name)
			}
			return run(in, ed)
		}
	}
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line
}

// The whole capability: a printable key reaches the shell, and what the shell
// hands back is the line.
func TestAPrintableKeyReachesTheShellsSelfInsert(t *testing.T) {
	var seen int
	got := typedWithSelfInsert(t, func(in Line, _ Actions) (Line, bool) {
		seen++
		// A wrapper that inserts something of its own rather than the key.
		return Line{Buffer: in.Buffer + "!", Cursor: len(in.Buffer) + 1}, true
	}, "ab\n")
	if seen != 2 {
		t.Errorf("the shell saw %d keystrokes, want 2 — one per printable key", seen)
	}
	if want := "!!"; got != want {
		t.Errorf("line = %q, want %q — what the shell returned is the line", got, want)
	}
}

// A shell with no such action leaves typing exactly as it was, which is every
// session without a highlighter and the path that must stay cheap.
func TestAShellWithNoSelfInsertTypesNormally(t *testing.T) {
	if got, want := typedWithSelfInsert(t, func(Line, Actions) (Line, bool) {
		return Line{}, false // no such widget
	}, "abc\n"), "abc"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
	// And a dialect that never named one does not ask at all.
	var out strings.Builder
	asked := false
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("abc\n"), &out
	e.runFunc = func(string, Line, Actions) (Line, bool) { asked = true; return Line{}, false }
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Error("the shell was asked though no dialect named a self-insert widget")
	}
	if line != "abc" {
		t.Errorf("line = %q, want abc", line)
	}
}

// The wrapper reaches the real insertion by asking the editor to perform it,
// and what gets inserted is the key the person actually pressed.
//
// This is the round trip a plugin's wrapper makes: `zle .self-insert` comes
// back through repl.Actions as WidgetSelfInsert, and nothing in that call says
// which key — the editor is holding it.
func TestPerformingSelfInsertInsertsTheKeyThatWasPressed(t *testing.T) {
	got := typedWithSelfInsert(t, func(in Line, ed Actions) (Line, bool) {
		out, ok := ed.Perform(WidgetSelfInsert, in)
		if !ok {
			t.Error("the editor declined to insert")
		}
		return out, true
	}, "hi\n")
	if want := "hi"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// A wrapper that performs the insertion twice gets the same key twice, which
// is what says the key is held rather than consumed by the first call.
func TestPerformingSelfInsertTwiceRepeatsTheKey(t *testing.T) {
	got := typedWithSelfInsert(t, func(in Line, ed Actions) (Line, bool) {
		out, _ := ed.Perform(WidgetSelfInsert, in)
		out, _ = ed.Perform(WidgetSelfInsert, out)
		return out, true
	}, "ab\n")
	if want := "aabb"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// A wrapper that commits the line while handling a printable key is obeyed,
// the way one wrapping accept-line is.
func TestASelfInsertWrapperCanCommitTheLine(t *testing.T) {
	got := typedWithSelfInsert(t, func(in Line, ed Actions) (Line, bool) {
		out, _ := ed.Perform(WidgetSelfInsert, in)
		out.Accept = true
		return out, true
	}, "xy")
	if want := "x"; got != want {
		t.Errorf("line = %q, want %q — the first key committed it", got, want)
	}
}

// The name is the dialect's, and the editor asks by it.
func TestTheSelfInsertNameIsTheDialects(t *testing.T) {
	var out strings.Builder
	var asked string
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("a\n"), &out
	e.selfInsert = "insert-the-thing"
	e.runFunc = func(name string, _ Line, _ Actions) (Line, bool) {
		asked = name
		return Line{}, false
	}
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if asked != "insert-the-thing" {
		t.Errorf("asked for %q, want the name the dialect gave", asked)
	}
}
