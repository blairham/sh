// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"context"
	"strings"
	"testing"
)

// The other direction: an action outside the editor reaching back into it.
//
// shellwidget_test.go pins the round trip — the line out, the line back. This
// pins what the action can *ask for* while it is holding the line, which is
// the seam editoractions.go adds. Nothing here names a shell; what the
// spellings are called is asserted in dialect/zsh, next to the measurement.

// typedReachingBack runs a line through an editor whose ^G is bound to a shell
// action, with the editor handed to that action so it can ask for something.
//
// history is what the session has already accepted, seeded directly because
// the walk is the action most of this file is about and a test that had to
// type three lines first would be asserting on the typing.
func typedReachingBack(
	t *testing.T, history []string, act func(Line, Actions) (Line, bool), keys string,
) (string, string) {
	t.Helper()
	return typedReachingBackStyled(t, EditorStyle{}, nil, history, act, keys)
}

// typedReachingBackStyled is typedReachingBack in an editor of one dialect's
// style, for an answer the dialects give differently. dialect, when not nil,
// sets an answer that lives outside EditorStyle on the editor once it is
// built — the search's, which is HistoryStyle's.
func typedReachingBackStyled(
	t *testing.T, style EditorStyle, dialect func(*editor), history []string,
	act func(Line, Actions) (Line, bool), keys string,
) (string, string) {
	t.Helper()
	var out strings.Builder
	s := Shell{
		Editor:      style,
		KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = history
	e.browsing = len(history)
	if dialect != nil {
		dialect(e)
	}
	// Straight into runFunc rather than through Shell.RunWidget, because the
	// handle is what is being tested and the context carriage in between has
	// a test of its own below.
	e.runFunc = func(_ string, in Line, ed Actions) (Line, bool) { return act(in, ed) }
	line, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatalf("%q: %v", keys, err)
	}
	return line, out.String()
}

// An action can ask the editor to walk history, and what comes back is the
// entry — which is the whole of why this seam exists. A plugin that fails to
// match its own query falls back to `zle up-line-or-history`, and a shell
// where that is a refusal has an up arrow that recalls nothing.
func TestAnActionCanAskTheEditorToWalkHistory(t *testing.T) {
	var walked []Line
	got, _ := typedReachingBack(t, []string{"echo one", "echo two"},
		func(in Line, ed Actions) (Line, bool) {
			for range 3 {
				out, ok := ed.Perform(WidgetPreviousHistory, in)
				if !ok {
					t.Error("the editor declined to walk history")
				}
				walked = append(walked, out)
				in = out
			}
			return in, true
		}, "\a\n")
	// Keys is the keystroke the widget was bound to, `^G`, carried on every
	// line the editor hands out — zsh's `$KEYS`.
	want := []Line{
		{Buffer: "echo two", Cursor: 8, Keys: "\a"},
		{Buffer: "echo one", Cursor: 8, Keys: "\a"},
		// The third step is at the top and the walk stays there. It is not an
		// error: an action that ran off the end is told the line, not told no.
		{Buffer: "echo one", Cursor: 8, Keys: "\a"},
	}
	if len(walked) != len(want) {
		t.Fatalf("walked %+v, want %+v", walked, want)
	}
	for i := range want {
		// The history number is the session's to count, and this editor
		// has none; see TestHistNoNumbersTheLineBeingEdited.
		walked[i].HistNo = 0
		if walked[i] != want[i] {
			t.Errorf("step %d = %+v, want %+v", i+1, walked[i], want[i])
		}
	}
	if want := "echo one"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// The action is the editor's own and not a second copy of it, which is what
// keeps a key bound to an action and a widget asking for it the same thing.
//
// A walk that recalled an entry and then had it edited leaves the edit behind
// at that step — measured in both real shells, and pinned in history_test.go
// for the key. Asking through the seam has to get the same behavior, and it
// does because it runs the same code: recall, change the line, walk on and
// walk back, and the change is still there.
func TestAskingForAnActionGetsTheEditorsOwn(t *testing.T) {
	var back Line
	typedReachingBack(t, []string{"first", "second"},
		func(in Line, ed Actions) (Line, bool) {
			in, _ = ed.Perform(WidgetPreviousHistory, in) // "second"
			in.Buffer, in.Cursor = in.Buffer+"XX", 8      // edit it
			in, _ = ed.Perform(WidgetPreviousHistory, in) // "first"
			back, _ = ed.Perform(WidgetNextHistory, in)   // back to the edit
			return back, true
		}, "\a\n")
	if want := (Line{Buffer: "secondXX", Cursor: 8, Keys: "\a"}); back != want {
		t.Errorf("walked back to %+v, want %+v — the edit must survive the walk", back, want)
	}
}

// The incremental search runs from inside an action, with its own keys, and
// what comes back is the line it found and how it ended.
//
// This used to be the one action the seam declined, on the reasoning that it
// has a read loop of its own and running it from a widget would re-enter the
// editor's. It does not: the search reads through the editor's buffer, and the
// key loop is waiting on the action rather than on the terminal. The refusal
// cost `C-r` to everyone running zsh-autosuggestions, whose wrapper calls the
// search by name (#5895).
//
// The key that ends the search is the key loop's, read after the action
// returns: Return runs the line the search found, which is what zsh 5.9.2
// does with the same wrapper, measured 2026-10-04.
func TestTheSearchRunsFromInsideAnAction(t *testing.T) {
	history := []string{"echo alpha one", "echo bravo two", "printf charlie"}
	cases := []struct {
		name      string
		keys      string
		interrupt bool
		want      Line
		accepted  string
	}{
		{
			"found, then Return", "\abra\r", false,
			Line{Buffer: "echo bravo two", Cursor: 5, Keys: "\r"},
			"echo bravo two",
		},
		{
			"an empty query", "\a\r", false,
			Line{Keys: "\r"},
			"",
		},
		{
			"failing, then Return", "ab\azzz\r", false,
			Line{Buffer: "ab", Cursor: 2, Status: 1, Keys: "\r"},
			"ab",
		},
		{
			"abandoned with C-g", "ab\abra\a\r", false,
			Line{Buffer: "ab", Cursor: 2, Status: 3, Keys: "\a"},
			"ab",
		},
		{
			"abandoned with C-c, where C-c is the search's", "ab\abra\x03\r", true,
			Line{Buffer: "ab", Cursor: 2, Status: 3},
			"ab",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var got Line
			var ok bool
			calls := 0
			line, _ := typedReachingBackStyled(t, EditorStyle{}, func(e *editor) { e.searchInterruptAborts = c.interrupt }, history,
				func(in Line, ed Actions) (Line, bool) {
					calls++
					got, ok = ed.Perform(WidgetSearchHistoryBackward, in)
					return got, true
				}, c.keys)
			if !ok {
				t.Fatal("the editor declined the search from inside an action")
			}
			if calls != 1 {
				t.Errorf("the action ran %d times, want once — the search's keys are its own", calls)
			}
			got.Last, got.HistNo = LastWidget{}, 0
			if got != c.want {
				t.Errorf("the search gave back %+v, want %+v", got, c.want)
			}
			if line != c.accepted {
				t.Errorf("accepted %q, want %q", line, c.accepted)
			}
		})
	}
}

// And `C-c` where it is the line's, which is bash's answer and the zero value:
// the search ends and the byte goes back to the key loop, which abandons the
// line — so the action's code after the call still runs first.
func TestAnInterruptInTheSearchIsTheLinesByDefault(t *testing.T) {
	var after bool
	var out strings.Builder
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing("ab\abra\x03"), &out
	e.history = []string{"echo bravo two"}
	e.browsing = 1
	e.runFunc = func(_ string, in Line, ed Actions) (Line, bool) {
		in, _ = ed.Perform(WidgetSearchHistoryBackward, in)
		after = true
		return in, true
	}
	if line, err := e.readLine(drawPrompt("$ ")); err == nil {
		t.Errorf("readLine = %q, want the line abandoned", line)
	}
	if !after {
		t.Error("the action's code after the search did not run")
	}
}

// Characters an action pushes back are read as though they were typed, and
// they are read before anything the terminal has already delivered.
func TestPushedCharactersAreReadNext(t *testing.T) {
	got, _ := typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		ed.PushKeys("XY")
		return in, true
	}, "ab\acd\n")
	if want := "abXYcd"; got != want {
		t.Errorf("line = %q, want %q — the pushed characters come before what follows", got, want)
	}
}

// Two pushes in one action come back newest first, each push's own characters
// in order.
//
// Measured 2026-09-12 against zsh 5.9.2 inside a widget: pushing `ab` and then
// `cd` leaves `cdab` on the line. Appending instead of pushing in front would
// produce `abcd`, which is the plausible wrong answer and the reason this is a
// test rather than an implementation detail.
func TestTwoPushesComeBackNewestFirst(t *testing.T) {
	got, _ := typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		ed.PushKeys("ab")
		ed.PushKeys("cd")
		return in, true
	}, "\a\n")
	if want := "cdab"; got != want {
		t.Errorf("line = %q, want %q", got, want)
	}
}

// A redisplay puts the line on the screen before the action has finished,
// which is what an action that is about to do something slow is asking for.
//
// The assertion is that the *buffer it was given* reached the terminal during
// the call, not that the screen looks a particular way: what a redraw writes
// is redraw's answer and it has tests of its own.
func TestARedisplayDrawsBeforeTheActionReturns(t *testing.T) {
	var duringCall string
	_, screen := typedReachingBack(t, nil, func(in Line, ed Actions) (Line, bool) {
		before := in
		before.Buffer, before.Cursor = "halfway", 7
		ed.Redisplay(before)
		// Nothing outside can see the terminal mid-call, so the action reads
		// it back the only way it can: whatever is on the screen now is what
		// the redisplay wrote, and the line it returns is different, so the
		// two cannot be confused at the end.
		duringCall = "halfway"
		return Line{Buffer: "final", Cursor: 5}, true
	}, "\a\n")
	if !strings.Contains(screen, duringCall) {
		t.Errorf("screen = %q, want it to have shown %q while the action was still running",
			screen, duringCall)
	}
	if !strings.Contains(screen, "final") {
		t.Errorf("screen = %q, want the line the action returned on it too", screen)
	}
}

// The handle reaches the shell through the context the widget's function runs
// under, which is the carriage editoractions.go chose and the thing a dialect
// depends on: what asks for an action is a builtin several frames inside that
// function, not the entry point the seam calls.
func TestTheHandleRidesTheWidgetsContext(t *testing.T) {
	var fromContext Actions
	var found bool
	var out strings.Builder
	s := Shell{
		KeyBindings: func(Keymap) map[string]Binding { return map[string]Binding{"\a": {Function: "w"}} },
		RunWidget: func(ctx context.Context, name string, in Line) (Line, bool) {
			fromContext, found = ActionsFrom(ctx)
			return in, true
		},
	}
	e := s.newEditor(t.Context(), nil)
	e.in, e.out = typing("\a\n"), &out
	e.runFunc = s.shellWidgets(t.Context(), nil)
	if _, err := e.readLine(drawPrompt("$ ")); err != nil {
		t.Fatal(err)
	}
	if !found || fromContext == nil {
		t.Fatal("no editor on the widget's context, so `zle end-of-line` could never reach one")
	}
	// And a context that never went through the seam has none, which is what
	// makes a dialect's refusal outside a widget correct without a flag.
	if _, ok := ActionsFrom(t.Context()); ok {
		t.Error("a plain context carried an editor")
	}
}
