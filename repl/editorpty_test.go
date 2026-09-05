// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
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
	screen  *syncBuffer // what the editor draws, including the prompt
	ran     *syncBuffer // what the commands printed
	errs    *syncBuffer
	done    chan error
	prompt  int
}

func newSession(t *testing.T) *session {
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
	se := &session{
		t: t, control: control, screen: screen, ran: ran, errs: errs,
		done: make(chan error, 1),
	}
	go func() { _, err := s.Run(t.Context()); se.done <- err }()
	return se
}

// typeLine waits for the next prompt and types one line at it.
func (s *session) typeLine(keys string) {
	s.t.Helper()
	s.prompt++
	waitFor(s.t, s.screen, "["+itoa(s.prompt)+"]", "the prompt")
	if _, err := s.control.WriteString(keys); err != nil {
		s.t.Fatal(err)
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
