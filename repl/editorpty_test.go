// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
)

// The editing keys, through a real terminal, in a real session.
//
// Everything else about the editor is tested by handing it a reader, which is
// the right way to ask what it does with a keystroke and says nothing about
// whether the keystroke reaches it. The editor is only built where there is a
// terminal; raw mode, the reads and the redraw are on the far side of that,
// and from a reader-driven test a key that never arrives looks exactly like a
// key that works.
//
// Two things make this reliable. Each line waits for the *next prompt* rather
// than for output: the shell puts the terminal back in its own line discipline
// to run a command and takes it into raw mode afterwards, and the kernel drops
// what is queued but unread across that change, so anything typed earlier is
// simply gone (#635). And the prompt carries the command number, so waiting
// for one is waiting for a prompt that has not been drawn before — the same
// prompt is redrawn at every keystroke, and a wait for its text alone would be
// answered by the line that has already been typed.

// session is a shell with a terminal on one end and a test on the other.
type session struct {
	t       *testing.T
	control *os.File
	// tty is the shell's end of the same pseudo-terminal, which is the one
	// whose line discipline the editor changes — a test that asks what mode
	// the session's terminal is in has to ask this one.
	tty    *os.File
	screen *syncBuffer // what the editor draws, including the prompt
	ran    *syncBuffer // what the commands printed
	errs   *syncBuffer
	done   chan error
	prompt int
}

func newSession(t *testing.T) *session { return newSessionWith(t, nil) }

// newSessionWith is the same terminal with one more thing said about the
// shell, for a test whose subject is a field the wiring carries across.
//
// A hook rather than a second copy of the setup: the pty, the runner and the
// prompt that counts commands are what make these tests reliable, and a second
// copy of them is a second thing to keep right.
func newSessionWith(t *testing.T, configure func(*Shell)) *session {
	t.Helper()
	control, tty, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = control.Close()
	})

	screen, ran, errs := &syncBuffer{}, &syncBuffer{}, &syncBuffer{}
	// The prompt counts the commands, so every one of them is a mark that has
	// not been on the screen before.
	r := newTestRunner(map[string]string{"PS1": `[\#]`})
	r.Stdout, r.Stderr = ran, errs
	s := Shell{
		Runner: r, In: tty, Out: screen, Err: errs, Name: "sh",
		Style: PromptStyle{Escape: '\\', Codes: map[rune]PromptField{'#': FieldCommandNumber}},
	}
	if configure != nil {
		configure(&s)
	}
	se := &session{
		t: t, control: control, tty: tty, screen: screen, ran: ran, errs: errs,
		done: make(chan error, 1),
	}
	go func() { _, err := s.Run(t.Context()); se.done <- err }()
	return se
}

// typeLine waits for the next prompt and types one line at it, a byte at a
// time.
//
// A byte at a time and not one write, which is the difference between typing
// and pasting and is load-bearing for what these tests assert. The editor draws
// once per byte only while each byte arrives on its own; input already in hand
// is drawn once when it runs out (#1742). A harness that wrote the whole line
// in one call therefore produced a single redraw, and every assertion about an
// *intermediate* state — the highlighter coloring an unclosed quotation as it
// is typed, the move back up to the prompt row on the second draw — silently
// stopped testing anything while still passing.
//
// Each byte waits for the screen to grow rather than for a fixed delay, so this
// is as fast as the editor is and does not race it. The wait is bounded and
// does not fail: a key that draws nothing is a real possibility and not this
// helper's business to judge.
func (s *session) typeLine(keys string) {
	s.t.Helper()
	s.prompt++
	waitFor(s.t, s.screen, "["+itoa(s.prompt)+"]", "the prompt")
	for i := 0; i < len(keys); i++ {
		drawn := s.screen.Len()
		if _, err := s.control.WriteString(keys[i : i+1]); err != nil {
			s.t.Fatal(err)
		}
		for waited := 0; waited < 200 && s.screen.Len() == drawn; waited++ {
			time.Sleep(time.Millisecond)
		}
	}
}

// end sends ^D at a fresh prompt, which is how a session is told to stop.
func (s *session) end() {
	s.t.Helper()
	s.prompt++
	waitFor(s.t, s.screen, "["+itoa(s.prompt)+"]", "the last prompt")
	if _, err := s.control.WriteString("\x04"); err != nil {
		s.t.Fatal(err)
	}
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		s.t.Fatal("the shell did not end on ^D")
	}
}

func TestEditingThroughATerminal(t *testing.T) {
	s := newSession(t)
	for _, c := range []struct{ name, keys, want string }{
		// A backspace, a word kill, and the killed word put back elsewhere.
		{"a word kill and a yank", "echo one twoo\x7f\x17three \x19\n", "one three two"},
		// Home and End as a terminal in application cursor mode sends them,
		// which is the mode a full-screen program leaves behind.
		{"home and end in application mode", "world\x1bOHecho hello \x1bOF!\n", "hello world!"},
		// The spelling several other terminals use, and the one that used to
		// leave a `~` in the middle of the command.
		{"home as a numbered key", "hello\x1b[1~echo \n", "hello"},
		// Word motion by the arrows with a modifier held, which is what a
		// terminal sends for Ctrl-Left.
		{"ctrl-left, then a forward word kill", "echo aa bb cc\x1b[1;5D\x1b[1;5Ddd \x1bd\n", "aa dd cc"},
		// And the width arithmetic, on characters the terminal draws double.
		{"editing a line of wide characters", "echo 日本語\x02\x02x\n", "日x本語"},
	} {
		t.Run(c.name, func(t *testing.T) {
			s.typeLine(c.keys)
			waitFor(t, s.ran, c.want, "the command's output")
		})
	}
	s.end()
}

// A key this does not act on must reach no command either.
//
// The same claim the reader-driven tests make, made again where the bytes come
// off a terminal, because this is the failure a person notices first: a stray
// `~` or `;5C` in the middle of a command is not a missing feature but a
// broken shell. Measured, both real shells have some of it — and one of the
// sequences here is a mouse click, which in bash puts a `!!` in the line that
// history expansion then turns into the previous command.
func TestAnUnknownKeyReachesNoCommand(t *testing.T) {
	s := newSession(t)
	// A page down, a function key, a mouse click, a cursor-position report and
	// a terminal's answer to a question something else asked it.
	const noise = "\x1b[6~\x1b[15~\x1b[M !!\x1b[24;80R\x1b[?1;2c"
	s.typeLine("echo " + noise + "ok\n")
	waitFor(t, s.ran, "ok", "the command's output")
	s.end()

	// Exactly `ok` and a newline: a byte that survived would have become
	// another argument, and one that survived in front of it would have made
	// the command something else.
	if got := s.ran.String(); got != "ok\n" {
		t.Errorf("the commands printed %q, want %q", got, "ok\n")
	}
	if got := s.errs.String(); got != "" {
		t.Errorf("the session complained: %q", got)
	}
}

// A key that has begun and not finished, through a terminal and with real time
// passing in the middle of it.
//
// This is the one claim a reader-driven test cannot make. Handed a string, an
// Escape and the byte after it arrive together however they were written; on a
// terminal they arrive when they are typed, and the question is what the
// editor does in between.
//
// Measured, all three real shells wait indefinitely: with `echo one two` on the
// line, an Escape and then a `b` typed six seconds later is still `M-b` in bash
// 5.3.15, bash 3.2.57 and zsh 5.9.2, and `\e[` with three seconds before the
// `A` is still Up. So there is no timeout here either — a shell that gave up on
// a half-read key would be the only one that did.
func TestAnEscapeWaitsForTheKeyItNames(t *testing.T) {
	s := newSession(t)
	s.typeLine("echo one two\x1b")
	// Long enough that any bounded wait a shell might have would have expired:
	// the two the manuals name are 500ms and 400ms.
	time.Sleep(700 * time.Millisecond)
	if _, err := s.control.WriteString("bX\n"); err != nil {
		t.Fatal(err)
	}
	// `M-b` went back over `two`, so the `X` landed in front of it. Had the
	// Escape been given up on, the `b` would have been typed at the end.
	waitFor(t, s.ran, "one Xtwo", "the command's output")
	s.end()
}

// And ^C is the way out of one, which is what keeps that wait from being a
// wedge.
//
// Measured, all three shells abandon the line on ^C however far into a key
// sequence they are: their editors leave the terminal's ISIG on, so the kernel
// makes that one keystroke a signal wherever the editor is. This editor takes
// the terminal fully raw — ^C has to be a byte for the line to be abandoned
// without racing a read already in progress — so the rescue is written down
// rather than inherited, and this is the test that it reaches the terminal.
func TestControlCGetsOutOfAHalfTypedKeyOnATerminal(t *testing.T) {
	s := newSession(t)
	// A bare Escape, a pause a person would notice, and then the way out.
	s.typeLine("echo abandoned\x1b")
	time.Sleep(300 * time.Millisecond)
	if _, err := s.control.WriteString("\x03"); err != nil {
		t.Fatal(err)
	}
	// The prompt comes back below the line that was given up on — and it is
	// the *same* prompt, because nothing ran and the command number counts
	// commands. So the wait is on the abandoned line having been left behind
	// rather than on a number, and the line typed after it goes in directly.
	waitFor(t, s.screen, "abandoned\r\n[1]", "a prompt below the abandoned line")
	if _, err := s.control.WriteString("echo recovered\n"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, s.ran, "recovered", "the command after the interrupt")
	if strings.Contains(s.ran.String(), "abandoned") {
		t.Errorf("the abandoned line ran: %q", s.ran.String())
	}
	s.end()
}

// `M-.` and `^_`, through a terminal, in the shape a person uses them.
func TestTheLastArgumentAndUndoThroughATerminal(t *testing.T) {
	s := newSession(t)
	// The line whose last argument the next one reaches back for.
	s.typeLine("echo first-arg second-arg\n")
	waitFor(t, s.ran, "first-arg second-arg", "the first line's output")
	// `echo ` and then the argument that was just typed, without typing it.
	s.typeLine("echo picked-\x1b.\n")
	waitFor(t, s.ran, "picked-second-arg", "the last argument, inserted")
	// A whole line killed by mistake and taken back.
	s.typeLine("echo restored-line\x15\x1f\n")
	waitFor(t, s.ran, "restored-line", "the line ^U took away")
	s.end()
}
