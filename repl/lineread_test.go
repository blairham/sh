// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// A read a command asked for: the line starts from text it supplied.
//
// Nothing here names a shell, for bindings_test.go's reason. What the builtin
// that asks for this does with the answer is dialect/zsh's; this is what the
// editor does with the request.

// edited runs one such read and hands back the line and the error.
func edited(t *testing.T, history []string, req interp.LineEdit, keys string) (string, string, error) {
	t.Helper()
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), &out
	e.history = history
	e.browsing = len(history)
	line, err := e.readValue(drawPrompt(req.Prompt), req)
	return line, out.String(), err
}

// The text is on the line with the cursor after it, so typing appends and the
// editor's own motions reach what was already there.
func TestAReadStartsFromTheTextItWasGiven(t *testing.T) {
	for _, c := range []struct{ name, initial, keys, want string }{
		{"typing appends", "hello", "XY\n", "helloXY"},
		// `^A` puts the cursor in front of the text that was handed over,
		// which is the assertion that the text is on the *line* rather than
		// merely drawn in front of it.
		{"and the motions reach it", "world", "\x01hello \n", "hello world"},
		{"a kill takes it back", "hello", "\x15bye\n", "bye"},
		{"and an empty start is a fresh line", "", "made\n", "made"},
	} {
		t.Run(c.name, func(t *testing.T) {
			got, _, err := edited(t, nil, interp.LineEdit{Initial: c.initial}, c.keys)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("line = %q, want %q", got, c.want)
			}
		})
	}
}

// The prompt is drawn in front of it, and an empty prompt draws nothing —
// which is the ordinary case and is not a prompt of one space.
func TestAReadDrawsThePromptItWasGiven(t *testing.T) {
	_, screen, err := edited(t, nil, interp.LineEdit{Prompt: "P ", Initial: "hi"}, "\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(screen, "P hi") {
		t.Errorf("screen = %q, want the prompt and the value in it", screen)
	}
	_, screen, err = edited(t, nil, interp.LineEdit{Initial: "hi"}, "\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(screen, "P") {
		t.Errorf("screen = %q, want no prompt at all", screen)
	}
}

// The history is reachable only where the request asked for it, and it is
// taken away rather than filtered.
//
// Measured 2026-09-18 through a pseudo-terminal against zsh 5.9.2: Up during
// such a read rings the bell and recalls nothing without the option, and
// recalls the session's own last command with it.
func TestAReadReachesTheHistoryOnlyWhenAsked(t *testing.T) {
	const up = "\x1b[A"
	got, _, err := edited(t, []string{"first", "second"},
		interp.LineEdit{Initial: "hi", History: true}, up+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "second"; got != want {
		t.Errorf("with the history, Up gave %q, want %q", got, want)
	}
	got, _, err = edited(t, []string{"first", "second"},
		interp.LineEdit{Initial: "hi"}, up+"\n")
	if err != nil {
		t.Fatal(err)
	}
	if want := "hi"; got != want {
		t.Errorf("without it, Up gave %q, want the line untouched at %q", got, want)
	}
}

// End of input on an empty line ends the read only where the request asked.
//
// **The default is the measured one and it is not the obvious one.** In zsh
// `^D` during such a read is `delete-char-or-list`, so on an empty line it
// offers to list every command rather than ending anything; only `-e` makes it
// end the read. This editor's `^D` is not that action, so without the option
// the key does nothing here — which is the same "it does not end the read",
// arrived at from the other side.
func TestEndOfInputEndsAReadOnlyWhenAsked(t *testing.T) {
	_, _, err := edited(t, nil,
		interp.LineEdit{Initial: "hi", EndOnEndOfInput: true}, "\x15\x04")
	if !errors.Is(err, io.EOF) {
		t.Errorf("with the option, ^D on an empty line gave %v, want end of input", err)
	}
	got, _, err := edited(t, nil, interp.LineEdit{Initial: "hi"}, "\x15\x04done\n")
	if err != nil {
		t.Fatalf("without it, ^D ended the read: %v", err)
	}
	if want := "done"; got != want {
		t.Errorf("line = %q, want %q — the key must not have ended the read", got, want)
	}
	// And the session's own `^D` is untouched: a read nobody seeded still
	// ends on it, which is how a shell is told to exit.
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.in, e.out = typing("\x04"), &out
	if _, err := e.readLine(drawPrompt("$ ")); !errors.Is(err, io.EOF) {
		t.Errorf("^D at a prompt gave %v, want end of input", err)
	}
}

// An abandoned read is reported as an interrupt rather than as a line.
func TestAnAbandonedReadIsAnInterrupt(t *testing.T) {
	_, _, err := edited(t, nil, interp.LineEdit{Initial: "hi"}, "\x03")
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("^C gave %v, want an interrupt", err)
	}
}

// The read leaves the session's own state where it found it, so the next
// prompt is an ordinary one.
//
// The history cursor is the one that would bite: a read without the option
// takes the history away, and a prompt after it that had lost the session's
// entries would have a dead Up key for the rest of the session.
func TestAReadLeavesTheSessionsHistoryAlone(t *testing.T) {
	var out strings.Builder
	e := Shell{}.newEditor(t.Context(), nil)
	e.history = []string{"first", "second"}
	e.browsing = len(e.history)
	e.in, e.out = typing("X\n\x1b[A\n"), &out
	if _, err := e.readValue(drawPrompt(""), interp.LineEdit{Initial: "hi"}); err != nil {
		t.Fatal(err)
	}
	got, err := e.readLine(drawPrompt("$ "))
	if err != nil {
		t.Fatal(err)
	}
	if want := "second"; got != want {
		t.Errorf("the prompt after the read recalled %q, want %q", got, want)
	}
}
