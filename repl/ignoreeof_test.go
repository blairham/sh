// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// ^D on an empty line under an ignore-EOF setting (#6239).
//
// Measured 2026-10-06 through a pseudo-terminal with a two-row prompt: zsh
// 5.9.2 under `setopt ignoreeof` rings, writes `zsh: use 'exit' to exit.`
// under the line and goes back to it, nine times, and on the tenth writes it
// and leaves; bash 5.3.20 under `set -o ignoreeof` ends the line, writes
// `Use "exit" to leave the shell.` and draws a fresh prompt, and IGNOREEOF
// says how many times. Before this, the option was recorded and nothing read
// it, so the first ^D ended the session in both.

const (
	zshRefusal  = "zsh: use 'exit' to exit."
	bashRefusal = `Use "exit" to leave the shell.`
)

// refusingEditor is an editor whose prompt read refuses ^D the given number of
// times, drawn the given way.
func refusingEditor(t *testing.T, keys string, out io.Writer, limit int, on, staysOnTheLine bool) *editor {
	t.Helper()
	refusal := bashRefusal
	if staysOnTheLine {
		refusal = zshRefusal
	}
	e := Shell{
		EndOfInputRefused: refusal,
		Editor:            EditorStyle{EndOfInputRefusalStaysOnTheLine: staysOnTheLine},
	}.newEditor(t.Context(), nil)
	e.in, e.out = typing(keys), out
	e.endOfInputRefusals = func() (int, bool) { return limit, on }
	return e
}

func TestARefusedEndOfInputStaysOnTheLineInZsh(t *testing.T) {
	var out strings.Builder
	e := refusingEditor(t, "\x04\x04echo\r", &out, 9, true, true)
	line, err := e.readLine(drawPrompt("top\nP> "))
	if err != nil {
		t.Fatalf("the read ended with %v; two refused ^D leave the read going "+
			"on and the line typed after them accepted. Wrote %q", err, out.String())
	}
	if line != "echo" {
		t.Errorf("the line is %q, want %q", line, "echo")
	}
	if got := strings.Count(out.String(), bell+zshRefusal); got != 2 {
		t.Errorf("the bell and the refusal were written %d times, want 2: %q", got, out.String())
	}
	if e.refusedInARow != 2 {
		t.Errorf("counted %d refusals, want 2", e.refusedInARow)
	}
}

func TestTheTenthEndOfInputEndsAZshSessionAfterSayingSo(t *testing.T) {
	var out strings.Builder
	e := refusingEditor(t, "\x04", &out, 9, true, true)
	e.refusedInARow = 9
	if _, err := e.readLine(drawPrompt("P> ")); !errors.Is(err, io.EOF) {
		t.Fatalf("the tenth ^D ended the read with %v, want io.EOF", err)
	}
	if !strings.Contains(out.String(), bell+zshRefusal) {
		t.Errorf("the tenth ^D said nothing; zsh writes the refusal on it "+
			"too, and then leaves. Wrote %q", out.String())
	}
}

func TestARefusedEndOfInputEndsTheReadForAFreshPromptInBash(t *testing.T) {
	var out strings.Builder
	e := refusingEditor(t, "\x04", &out, 1, true, false)
	if _, err := e.readLine(drawPrompt("P> ")); !errors.Is(err, ErrEndOfInputRefused) {
		t.Fatalf("the first ^D ended the read with %v, want ErrEndOfInputRefused", err)
	}
	if strings.Contains(out.String(), bashRefusal) {
		t.Errorf("the editor wrote the refusal; in bash's shape it is the "+
			"session's, after anything pending is reported. Wrote %q", out.String())
	}
	// And IGNOREEOF=1 lets the second one through.
	e.in = typing("\x04")
	if _, err := e.readLine(drawPrompt("P> ")); !errors.Is(err, io.EOF) {
		t.Fatalf("the second ^D under a count of one ended the read with %v, want io.EOF", err)
	}
}

func TestEndOfInputIsNotRefusedWithTheSettingOffOrOutsideAPrompt(t *testing.T) {
	var out strings.Builder
	e := refusingEditor(t, "\x04", &out, 10, false, false)
	if _, err := e.readLine(drawPrompt("P> ")); !errors.Is(err, io.EOF) {
		t.Errorf("with the setting off ^D ended the read with %v, want io.EOF", err)
	}
	// A read nobody set the question for — a command's, not the prompt's.
	e.in = typing("\x04")
	e.endOfInputRefusals = nil
	if _, err := e.readLine(drawPrompt("P> ")); !errors.Is(err, io.EOF) {
		t.Errorf("a read with no refusal asked of it ended with %v, want io.EOF", err)
	}
}

// TestTheIgnoreEOFCountReadsDigitsAlone is the IGNOREEOF grid, measured on
// bash 5.3.20: the count is how many ^D are refused before one ends the
// session.
func TestTheIgnoreEOFCountReadsDigitsAlone(t *testing.T) {
	for _, c := range []struct {
		value string
		want  int
	}{
		{"3", 3},
		{"03", 3},
		{"0", 0},
		{"10", 10},
		{"", 10},
		{"abc", 10},
		{"-1", 10},
		{"+2", 10},
		{" 2", 10},
		{"2 ", 10},
		{"3x", 10},
		{"2.5", 10},
		{"0x2", 10},
	} {
		if got := endOfInputCount(c.value, 10); got != c.want {
			t.Errorf("IGNOREEOF=%q counts %d, want %d", c.value, got, c.want)
		}
	}
}

// TestABashSessionRefusesEndOfInputAndGoesOn drives the whole loop at a
// terminal: the refusal is on a row of its own after the ended line, the
// session draws the prompt again, a command run in between starts the count
// again, and with IGNOREEOF=1 the second ^D in a row ends it.
func TestABashSessionRefusesEndOfInputAndGoesOn(t *testing.T) {
	control, tty := openTerminal(t)
	screen := &syncBuffer{}
	r := newTestRunner(map[string]string{"PS1": "top\nP> ", "IGNOREEOF": "1"})
	r.Stdout, r.Stderr = screen, screen
	s := Shell{
		Runner: r, In: tty, Out: screen, Err: screen,
		Name: "bash", Leaving: "exit",
		EndOfInputRefused: bashRefusal,
		Editor:            EditorStyle{IgnoreEndOfInputParameter: "IGNOREEOF", EndOfInputRefusals: 10},
	}
	done := make(chan error, 1)
	go func() { _, err := s.Run(t.Context()); done <- err }()

	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
	}
	// promptAfter waits for a prompt drawn after the last place mark is on
	// the screen. The prompt has to be there before the next key is sent:
	// the shell takes the terminal into raw mode to read, and the kernel
	// drops what is queued but unread across that change (#635).
	promptAfter := func(mark string) {
		t.Helper()
		for deadline := time.Now().Add(20 * time.Second); ; {
			raw := screen.String()
			if at := strings.LastIndex(raw, mark); at >= 0 && strings.Contains(raw[at+len(mark):], "P> ") {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("no prompt after %q. Wrote %q", mark, raw)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	stillRunning := func(what string) {
		t.Helper()
		select {
		case err := <-done:
			t.Fatalf("the session ended on %s (%v). Wrote %q", what, err, screen.String())
		case <-time.After(200 * time.Millisecond):
		}
	}

	waitFor(t, screen, "P> ", "the prompt")
	send("\x04")
	promptAfter(bashRefusal)
	stillRunning("the first ^D; IGNOREEOF=1 refuses one")

	// A command, which starts the count again: without it the next ^D
	// would be the second in a row and would end the session.
	send("echo ran-$((6 * 7))\r")
	promptAfter("ran-42")
	send("\x04")
	waitForCount(t, screen, bashRefusal, 2)
	promptAfter(bashRefusal)
	stillRunning("the first ^D after a command; running one starts the count again")

	send("\x04")
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("the second ^D in a row did not end the session. Wrote %q", screen.String())
	}

	// On the bytes rather than on a modeled screen: the refusal goes to the
	// error stream in the terminal's own line discipline, where the kernel
	// turns its newline into a return and a line feed, and this fixture's
	// error stream is a buffer with no kernel in front of it.
	raw := screen.String()
	if !strings.Contains(raw, "P> \r\n"+bashRefusal+"\n") {
		t.Errorf("the refusal is not on a row of its own after the ended "+
			"line; bash writes `P> `, ends the row, then the refusal. Wrote %q", raw)
	}
}

// waitForCount waits for text to be on the screen n times.
func waitForCount(t *testing.T, screen *syncBuffer, text string, n int) {
	t.Helper()
	for deadline := time.Now().Add(20 * time.Second); strings.Count(screen.String(), text) < n; {
		if time.Now().After(deadline) {
			t.Fatalf("%q is on the screen %d times, want %d. Wrote %q",
				text, strings.Count(screen.String(), text), n, screen.String())
		}
		time.Sleep(10 * time.Millisecond)
	}
}
