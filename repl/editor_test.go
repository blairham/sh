// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"testing"
)

// The editor is tested by feeding it bytes rather than by driving a terminal.
//
// It reads from an io.Reader and draws to an io.Writer, so a string and a
// buffer are the whole harness — no pty, no timing, no raw mode. That is worth
// the small indirection: a pty test has to decide when the other end has
// finished writing, and gets it wrong on a loaded machine.

// typed runs one line's worth of input through a fresh editor.
func typed(t *testing.T, keys string) (string, string, error) {
	t.Helper()
	var out strings.Builder
	e := &editor{in: strings.NewReader(keys), out: &out}
	line, err := e.readLine("$ ")
	return line, out.String(), err
}

// typedMarking is typed with a dialect's mark for an abandoned line.
func typedMarking(t *testing.T, mark, keys string) (string, string, error) {
	t.Helper()
	var out strings.Builder
	e := &editor{in: strings.NewReader(keys), out: &out, interrupt: mark}
	line, err := e.readLine("$ ")
	return line, out.String(), err
}

func TestTypingALine(t *testing.T) {
	for _, c := range []struct{ keys, want string }{
		{"echo hi\r", "echo hi"},
		{"echo hi\n", "echo hi"},
		{"\r", ""},
		// A character outside ASCII arrives as several bytes and is one
		// rune in the line, not three.
		{"echo é\r", "echo é"},
		{"日本\r", "日本"},
	} {
		line, _, err := typed(t, c.keys)
		if err != nil {
			t.Errorf("%q: %v", c.keys, err)
			continue
		}
		if line != c.want {
			t.Errorf("%q gave %q, want %q", c.keys, line, c.want)
		}
	}
}

func TestEditingALine(t *testing.T) {
	for _, c := range []struct{ name, keys, want string }{
		{"backspace", "abcXX\x7f\x7f\r", "abc"},
		{"backspace at the start does nothing", "\x7f\x7fab\r", "ab"},
		{"delete forward with ^D", "abc\x02\x02\x04\r", "ac"},
		{"^D on an empty line is not a delete", "", ""},
		{"left then insert", "ac\x1b[Db\r", "abc"},
		{"^B then insert", "ac\x02b\r", "abc"},
		{"right after left", "ac\x1b[D\x1b[Dx\r", "xac"},
		{"^A goes to the start", "bc\x01a\r", "abc"},
		{"^E goes to the end", "bc\x01a\x05d\r", "abcd"},
		{"home and end", "bc\x1b[Ha\x1b[Fd\r", "abcd"},
		{"^K cuts to the end", "abcdef\x01\x06\x06\x06\x0b\r", "abc"},
		{"^U cuts to the start", "abcdef\x01\x06\x06\x06\x15\r", "def"},
		{"^W cuts a word", "one two\x17\r", "one "},
		{"^W cuts trailing spaces too", "one two   \x17\r", "one "},
		{"delete key", "abc\x01\x1b[3~\r", "bc"},
		// A control character with no meaning here is dropped rather than
		// put in the line: the parser must never see a byte nobody typed.
		{"an unhandled control byte", "a\x1cb\r", "ab"},
		// An escape sequence this does not know is dropped whole.
		{"an unknown escape", "a\x1b[Zb\r", "ab"},
	} {
		line, _, err := typed(t, c.keys)
		if err != nil && !errors.Is(err, io.EOF) {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if line != c.want {
			t.Errorf("%s: gave %q, want %q", c.name, line, c.want)
		}
	}
}

// ^C abandons the line and ^D on an empty one ends the session. They are
// different answers and the shell acts on each differently.
func TestInterruptAndEndOfInput(t *testing.T) {
	line, out, err := typed(t, "half a line\x03")
	if !errors.Is(err, ErrInterrupted) {
		t.Errorf("^C gave %v, want ErrInterrupted", err)
	}
	if line != "" {
		t.Errorf("^C kept %q, want the line abandoned", line)
	}
	// No mark unless the dialect asks for one: two of the four draw `^C`
	// after an abandoned line and two draw nothing, so the editor holds no
	// answer of its own.
	if strings.Contains(out, "^C") {
		t.Errorf("^C drew %q, want no mark from an editor told of none", out)
	}
	if _, out, _ := typedMarking(t, "^C", "half a line\x03"); !strings.Contains(out, "^C") {
		t.Errorf("drew %q, want the mark the dialect asked for", out)
	}
	if _, out, _ := typedMarking(t, "<int>", "half a line\x03"); !strings.Contains(out, "<int>") {
		t.Errorf("drew %q, want the mark the dialect asked for", out)
	}
	if _, _, err := typed(t, "\x04"); !errors.Is(err, io.EOF) {
		t.Errorf("^D on an empty line gave %v, want io.EOF", err)
	}
	// With something typed, ^D is a forward delete and not an exit.
	if _, _, err := typed(t, "ab\x01\x04\r"); err != nil {
		t.Errorf("^D with text gave %v, want it to keep reading", err)
	}
}

// The history is walked with the arrows, and the line being typed is put aside
// rather than lost.
func TestHistory(t *testing.T) {
	var out strings.Builder
	e := &editor{out: &out}
	e.remember("first")
	e.remember("second")

	e.in = strings.NewReader("\x1b[A\r")
	if line, _ := e.readLine("$ "); line != "second" {
		t.Errorf("one step back gave %q, want second", line)
	}
	e.in = strings.NewReader("\x1b[A\x1b[A\r")
	if line, _ := e.readLine("$ "); line != "first" {
		t.Errorf("two steps back gave %q, want first", line)
	}
	// Past the oldest is a floor, not a wrap.
	e.in = strings.NewReader("\x1b[A\x1b[A\x1b[A\x1b[A\r")
	if line, _ := e.readLine("$ "); line != "first" {
		t.Errorf("four steps back gave %q, want first", line)
	}
	// Forward again brings back what was being typed.
	e.in = strings.NewReader("half\x1b[A\x1b[B\r")
	if line, _ := e.readLine("$ "); line != "half" {
		t.Errorf("back then forward gave %q, want the typed line returned", line)
	}
	// ^P and ^N are the same two movements.
	e.in = strings.NewReader("\x10\x10\x0e\r")
	if line, _ := e.readLine("$ "); line != "second" {
		t.Errorf("^P^P^N gave %q, want second", line)
	}
}

// What is worth keeping in the history, and what is only clutter.
func TestWhatTheHistoryKeeps(t *testing.T) {
	e := &editor{}
	e.remember("one")
	e.remember("one")
	e.remember("")
	e.remember("   ")
	e.remember("two")
	if len(e.history) != 2 || e.history[0] != "one" || e.history[1] != "two" {
		t.Errorf("history is %q, want [one two]", e.history)
	}
}

// The prompt and the line are redrawn together, because the editor owns what
// is on the screen — the terminal echoes nothing in raw mode.
func TestTheLineIsDrawn(t *testing.T) {
	_, out, _ := typed(t, "hi\r")
	if !strings.Contains(out, "$ hi") {
		t.Errorf("drew %q, want the prompt and the line", out)
	}
	// With the cursor left of the end, the last thing written moves it back.
	_, out, _ = typed(t, "abc\x1b[D\r")
	if !strings.Contains(out, "\x1b[1D") {
		t.Errorf("drew %q, want the cursor moved back one", out)
	}
}

// Tab completes, and a second Tab lists — which needs the editor to know the
// previous keystroke was a Tab and nothing else.
func TestTabAndSecondTab(t *testing.T) {
	c := fakeCompleter{paths: []string{"apple.txt", "apricot.txt"}}
	var out strings.Builder
	e := &editor{in: strings.NewReader("echo ap\t\r"), out: &out, comp: c}
	if line, err := e.readLine("$ "); err != nil || line != "echo ap" {
		t.Fatalf("one Tab gave %q %v, want the line unchanged", line, err)
	}
	if strings.Contains(out.String(), "apricot") {
		t.Error("one Tab listed the matches, want it to wait for the second")
	}

	out.Reset()
	e = &editor{in: strings.NewReader("echo ap\t\t\r"), out: &out, comp: c}
	if _, err := e.readLine("$ "); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "apple.txt") || !strings.Contains(out.String(), "apricot.txt") {
		t.Errorf("two Tabs drew %q, want both matches listed", out.String())
	}

	// Anything between them is not two Tabs in a row.
	out.Reset()
	e = &editor{in: strings.NewReader("echo ap\tx\x7f\t\r"), out: &out, comp: c}
	if _, err := e.readLine("$ "); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "apricot") {
		t.Errorf("drew %q, want a keystroke between the Tabs to have reset it", out.String())
	}
}

// With no completer, Tab does nothing rather than crashing — which is what an
// embedded caller that never set one gets.
func TestTabWithNoCompleter(t *testing.T) {
	line, _, err := typed(t, "echo a\t\r")
	if err != nil {
		t.Fatal(err)
	}
	if line != "echo a" {
		t.Errorf("got %q, want the line untouched", line)
	}
}
