// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"
	"strings"
	"testing"
	"time"
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
	// ended says the shell's goroutine has been waited for, so the cleanup
	// registered by newSessionWith has nothing left to do. Written and read on
	// the test's own goroutine, which is the only one that ends a session.
	ended bool
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
	control, tty := openTerminal(t)

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
	// The shell goroutine has to be finished with the terminal before the
	// terminal is closed, and most of these tests end without saying so: the
	// last thing they assert is a command's output, and the shell is drawing
	// the next prompt when the test body returns.
	//
	// Registered *after* openTerminal, which is what makes the order right —
	// cleanups run last-registered-first, so this one stops the shell and the
	// one that closes the pty runs after it. Without it the close raced the
	// shell taking the terminal into raw mode for the next line: a read of the
	// same `*os.File` the cleanup was closing, reported on the Linux leg of
	// PR #4317 as a data race between `internal/tty.setMode` and
	// `os.(*File).Close` (#4325).
	t.Cleanup(se.stop)
	return se
}

// stop ends the shell and waits for its goroutine, and does nothing where a
// test has already done that for itself.
//
// Closing the *control* end is what ends it: the shell's reads of its own end
// then fail, which is the same end of input a terminal going away gives it.
// Sending `^D` would not do — a session may be sitting on a half-typed line,
// where the byte is a delete rather than an end of input.
//
// The wait is the point of the whole thing, and it is bounded: a shell that
// does not come back is a defect and the timeout says so, where a cleanup that
// blocked would leave the package's timeout to report it as something else
// entirely.
func (s *session) stop() {
	if s.ended {
		return
	}
	s.ended = true
	_ = s.control.Close()
	select {
	case <-s.done:
	case <-time.After(20 * time.Second):
		s.t.Error("the shell did not stop when its terminal was closed")
	}
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
	s.typeKeys(keys)
}

// typeKeys types at whatever the session is already showing, for a test that
// has typed once and is going on typing into the same line.
//
// The wait for a prompt belongs to the first keystroke of a line and not to
// the rest of them: a second wait would be answered only by a prompt that has
// not been drawn, and a half-typed line has not produced one.
func (s *session) typeKeys(keys string) {
	s.t.Helper()
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

// shown is what a terminal of the fixture's size would be showing, colors and
// all, after everything the session has drawn.
//
// **The screen and not the bytes.** An incremental redraw writes whichever of
// several equivalent sequences is shortest for the change in hand, so a test
// naming one of them is asserting a coincidence — and for as long as every
// fixture here ran at width 0 the coincidence being asserted was the one the
// whole-line draw produced, which is a path no session with a terminal takes
// (#2627). See screenmodel_test.go.
func (s *session) shown() *screen { return shownBy(fixtureCols, s.screen.String()) }

// row is one line of what the screen is showing, with the colors written back
// into it.
func (s *session) row(n int) string {
	s.t.Helper()
	rows := strings.Split(s.shown().styledText(), "\n")
	if n >= len(rows) {
		s.t.Fatalf("the screen has %d row(s), so there is no row %d:\n%q", len(rows), n, s.screen.String())
	}
	return rows[n]
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
		// Waited for here, so the cleanup that waits for it has nothing left
		// to do. See session.stop.
		s.ended = true
		if err != nil {
			s.t.Fatal(err)
		}
	case <-time.After(20 * time.Second):
		s.t.Fatal("the shell did not end on ^D")
	}
}

// A session stops when its terminal's far end closes, which is what the
// cleanup every session registers relies on.
//
// The race this is written against was between the *shell* taking the terminal
// into raw mode for the next line and the *test's* cleanup closing that same
// `*os.File` — reported on the Linux leg of PR #4317, green on a re-run, which
// is how a real intermittent hides. Nothing in the shipped path was wrong: the
// fixture was closing a terminal a live shell still held (#4325).
//
// This asserts the half that can be asserted without a race detector and
// without loading the machine: that the stop returns at all. A shell left
// drawing a prompt has to come back when the far end goes away, or every one
// of these tests would pay a twenty-second timeout on the way out.
func TestASessionStopsWhenItsTerminalCloses(t *testing.T) {
	s := newSession(t)
	s.typeLine("echo hi\r")
	waitFor(t, s.ran, "hi\n", "the command's output")
	// Mid-prompt, which is where the tests that never say `end()` leave it.
	s.stop()
	if !s.ended {
		t.Error("the session is not marked ended, so the cleanup would wait for it twice")
	}
	// And a second stop is a no-op rather than a second wait on a channel
	// nothing will write to again.
	s.stop()
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

// A session on a real terminal takes the minimal redraw, and the fixture's
// terminal is really the size the fixture says it is.
//
// This is the instrument's own check and it is here because the instrument was
// the defect. Every fixture in this package ran at width 0 (#2627), which is
// the redraw's "nothing known about the terminal" branch: the whole line, the
// prompt with it, once per keystroke. Everything these tests reported green
// was about that branch, and the branch a person is on — the O(change) repaint
// — was exercised by no session at all. Nothing said so, because a terminal
// that will not give its size is a case the editor handles rather than an
// error it reports.
//
// The prompt is what tells the two apart, and it is the right marker rather
// than a convenient one: not rewriting the prompt for a keystroke is most of
// why the minimal redraw is cheaper, and it is what zsh does. A prompt drawn
// once for the line is the repaint path; a prompt per character is the other
// one.
func TestASessionThroughATerminalRedrawsOnlyWhatChanged(t *testing.T) {
	s := newSession(t)
	if rows, cols := terminalSize(s.tty); rows != fixtureRows || cols != fixtureCols {
		t.Fatalf("the fixture's terminal is %dx%d, want %dx%d — a session drawing for a terminal "+
			"nobody has is not the session anybody runs", rows, cols, fixtureRows, fixtureCols)
	}

	const keys = "echo one two three"
	s.typeLine(keys)
	if got, want := s.row(0), "[1]"+keys; got != want {
		t.Errorf("the screen shows %q, want %q", got, want)
	}
	if n := strings.Count(s.screen.String(), "[1]"); n != 1 {
		t.Errorf("the prompt was written %d times for %d keystrokes, want once: a keystroke that "+
			"rewrites the prompt is the whole-line draw, which is the width-0 path", n, len(keys))
	}

	s.typeKeys("\n")
	waitFor(t, s.ran, "one two three", "the command's output")
	s.end()
}
