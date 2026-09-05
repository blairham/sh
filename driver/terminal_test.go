// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pty"
)

// The prompt decision is "is this a terminal", and a character device is not
// the same question.
//
// `sh < /dev/null` is the cron, systemd, CI and harness invocation, and the
// whole point of redirecting there is that the shell waits for nobody. The
// null device is a character device, so the mode-bit test said terminal, the
// line editor asked it for raw mode, the ioctl answered ENOTTY, and every
// binary in the tree exited 2 with
//
//	sh: operation not supported by device
//
// Measured across the panel — see docs/spec/invocation.md — not one of the
// four prompts on the null device, on a regular file, on a pipe or on a closed
// descriptor. The grid is unanimous, so this is a fact about shells rather
// than an axis.
func TestNoStdinShortOfATerminalIsAPrompt(t *testing.T) {
	for _, tc := range []struct {
		name string
		open func(t *testing.T) *os.File
	}{
		{
			// The one in the issue, and the one every harness hands a child.
			name: "the null device",
			open: func(t *testing.T) *os.File { return openFile(t, os.DevNull) },
		},
		{
			// A second character device, to show the fix is not a special
			// case for one path. Opened and never read: it is endless, and
			// what a shell does with an endless script is a different
			// question from whether it prompts.
			name: "another character device",
			open: func(t *testing.T) *os.File { return openFile(t, "/dev/zero") },
		},
		{
			name: "a regular file",
			open: func(t *testing.T) *os.File {
				path := filepath.Join(t.TempDir(), "in.sh")
				if err := os.WriteFile(path, []byte("echo hello\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				return openFile(t, path)
			},
		},
		{
			name: "a pipe",
			open: func(t *testing.T) *os.File { return pipeWith(t, "echo hello\n") },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sh := shell()
			sh.Stdin = tc.open(t)
			if driver.Interactively(sh, false) {
				t.Errorf("Interactively = true, want false for %s", tc.name)
			}
		})
	}
}

// The whole of the reported bug, through the front end rather than at the
// seam: a shell handed the null device and nothing to do reads no commands,
// says nothing and exits 0.
func TestTheNullDeviceIsAnEmptyScriptAndNotAPrompt(t *testing.T) {
	sh := shell()
	sh.Stdin = openFile(t, os.DevNull)
	out, errs, code := runArgs(t, sh, "testsh")
	if code != 0 {
		t.Errorf("status = %d, want 0", code)
	}
	if out != "" || errs != "" {
		t.Errorf("out = %q, err = %q, want both empty", out, errs)
	}
}

// And the half that only a terminal can prove.
//
// Everything above is a negative, and a test made only of negatives passes
// just as well for a shell that never prompts at all — which is the failure
// on the other side of this fix and is no better than the one being fixed.
// So: a real pseudo-terminal, and the same call the front end makes.
func TestATerminalOnStandardInputIsAPrompt(t *testing.T) {
	_, tty := terminal(t)
	sh := shell()
	sh.Stdin = tty
	if !driver.Interactively(sh, false) {
		t.Error("Interactively = false with a terminal on standard input, want true")
	}
	// The other half of the rule: something to run wins over the terminal.
	// `sh script.sh` at a terminal runs the script and does not prompt, in
	// all four.
	if driver.Interactively(sh, true) {
		t.Error("Interactively = true with work to do, want false")
	}
}

// The prompt reaches a real terminal, end to end.
//
// The seam test above proves the decision; this proves what the decision buys.
// It is the only shape that exercises the line editor at all — the editor
// exists only where there is a terminal, so every other test in this package
// runs the plain loop instead — and it is what would have caught the reported
// bug from the other direction, since the editor is what asked the null device
// for raw mode and failed.
func TestAPromptAtATerminalReadsAndRunsALine(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // history and startup files, never the user's

	control, tty := terminal(t)
	sh := shell()
	// All three streams on the terminal, which is what a session has and
	// what lets this wait on what the shell drew rather than on a clock.
	sh.Stdin, sh.Stdout, sh.Stderr = tty, tty, tty

	drawn := watch(t, control)
	done := make(chan int, 1)
	go func() { done <- driver.MainArgs(sh, []string{"testsh"}) }()

	// Wait for the prompt before typing. Bytes written into a terminal
	// before the shell has put it in raw mode are read under the line
	// discipline instead, and ^D there is consumed as an end-of-file marker
	// rather than delivered as the byte the editor reads.
	drawn.awaitReadyForInput(t)
	// Typed as an expression so that the echo of the line and the output of
	// running it are different text: waiting for `mark-42` cannot be
	// satisfied by the shell merely drawing back what was typed.
	write(t, control, "echo mark-$((6 * 7))\r")
	drawn.await(t, "mark-42")
	// And the same rule again on the way out: the output says the command
	// ran, the *next prompt* says the shell is reading.
	drawn.endSession(t, control)

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("status = %d, want 0", code)
		}
	case <-time.After(sessionBudget):
		_ = control.Close() // unblock the read the shell is sitting in
		t.Fatalf("the session did not end; drawn so far: %q", drawn.text())
	}
}

// terminal is a pseudo-terminal pair, or a skip where the platform has none.
func terminal(t *testing.T) (control, tty *os.File) {
	t.Helper()
	control, tty, err := pty.Open()
	if errors.Is(err, pty.ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = tty.Close()
		_ = control.Close()
	})
	return control, tty
}

func openFile(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func write(t *testing.T, f *os.File, s string) {
	t.Helper()
	if _, err := f.WriteString(s); err != nil {
		t.Fatalf("writing %q to the terminal: %v", s, err)
	}
}

// sessionBudget bounds a wait on a pseudo-terminal.
//
// It is there to turn a hang into a failure, and it is deliberately not
// tuned to what the work costs: the work costs a hundredth of a second, and
// nothing that happens on a loaded runner turns that into ten. Measured on
// Linux under `-race`, one core and eight spinning processes, 1000 runs of
// the prompt tests: 995 finished between 0.01s and 0.11s, and the five that
// did not were at 10.01s — the deadline exactly, with the shell's own output
// already on the screen. That is the shape of a lost keystroke and not of a
// slow machine, so raising the number would only make each failure cost more
// and would fix none of them.
//
// One constant, in one place, so the next person changing it has this
// paragraph in front of them rather than a literal in three tests.
const sessionBudget = 10 * time.Second

// screen collects everything the shell draws, so a test can wait on a mark in
// it rather than on a duration.
//
// It never forgets anything: text() is what a failure prints, and a buffer
// that had been cleared between waits would report a screen nobody saw. What
// moves instead is a cursor — see await.
type screen struct {
	mu   sync.Mutex
	buf  strings.Builder
	seen int
}

// watch drains a stream into a screen. It takes an io.Reader rather than the
// pseudo-terminal itself so that the waiting can be tested without one.
func watch(t *testing.T, control io.Reader) *screen {
	t.Helper()
	s := &screen{}
	go func() {
		b := make([]byte, 4096)
		for {
			n, err := control.Read(b)
			if n > 0 {
				s.mu.Lock()
				s.buf.Write(b[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	return s
}

func (s *screen) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// seek reports whether the mark has been drawn *since the last one was*, and
// moves the cursor past it if so.
//
// The cursor is the whole point. A session draws the same prompt again after
// every line, so a plain search of everything drawn answers "yes" from the
// first prompt forever, and a test meaning "the shell is prompting again" is
// told so before the line it typed has even run. Advancing past each mark as
// it is found is how a wait stays a wait; clearing the buffer would do the
// same job and take the failure message with it.
func (s *screen) seek(mark string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := strings.Index(s.buf.String()[s.seen:], mark)
	if i < 0 {
		return false
	}
	s.seen += i + len(mark)
	return true
}

// await blocks until the mark has been drawn, and fails rather than hanging
// if it never is.
func (s *screen) await(t *testing.T, mark string) {
	t.Helper()
	deadline := time.Now().Add(sessionBudget)
	for time.Now().Before(deadline) {
		if s.seek(mark) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waited for %q; drawn so far: %q", mark, s.text())
}

// awaitReadyForInput waits until the session is back at a prompt, which is
// the only moment at which it is safe to type at it.
//
// A shell puts the terminal back in its own line discipline to run a command
// and takes it into raw mode again afterwards, and the kernel drops whatever
// is queued but unread when the discipline changes. So a byte written between
// the command's output appearing and the next prompt being drawn is simply
// gone: ^D sent there is swallowed, the editor goes on waiting for input that
// will never come, and the test fails at its deadline with the shell's own
// output already on the screen and the session still alive. That is exactly
// how it failed on the runner (#635).
//
// The prompt is the mark to wait on because the shell draws it after raw mode
// is restored and immediately before it reads — the two events cannot be
// reordered, which is what makes this a synchronization rather than a guess.
func (s *screen) awaitReadyForInput(t *testing.T) {
	t.Helper()
	s.await(t, "$ ")
}

// endSession types the ^D that ends a session, once the shell is reading.
//
// The wait belongs in here rather than at each call site: three tests end a
// session and every one of them had the wait missing, so the way to keep a
// fourth from repeating it is to leave no way to type the byte without it.
func (s *screen) endSession(t *testing.T, control *os.File) {
	t.Helper()
	s.awaitReadyForInput(t)
	write(t, control, "\x04") // ^D on an empty line ends the session
}

// TestAWaitIsNotSatisfiedByAMarkItAlreadySaw is the fix, stated where it can
// be checked without a race.
//
// A session draws the same prompt after every line, so "wait for the prompt"
// searched over everything drawn is answered by the *first* one for the rest
// of the run. A test that then types is typing at a shell that may still be
// running the previous command, with the terminal in its own line discipline
// and nothing reading — and the kernel drops what is queued but unread when
// the discipline changes back. That is how ^D went missing on the runner, and
// it is why this is about a cursor rather than about a longer deadline.
func TestAWaitIsNotSatisfiedByAMarkItAlreadySaw(t *testing.T) {
	drawn := &screen{}
	drawn.buf.WriteString("$ ")

	if !drawn.seek("$ ") {
		t.Fatal("the first prompt was not found at all")
	}
	if drawn.seek("$ ") {
		t.Error("the same prompt answered twice, so a later wait would not wait")
	}

	// The line is echoed, it runs, and the shell prompts again.
	drawn.buf.WriteString("echo mark-42\r\nmark-42\r\n$ ")
	if !drawn.seek("mark-42") {
		t.Error("the echo of the line was not found")
	}
	if !drawn.seek("mark-42") {
		t.Error("the output of the line was not found; the echo hid it")
	}
	if !drawn.seek("$ ") {
		t.Error("the second prompt was not found")
	}
	if drawn.seek("$ ") {
		t.Error("a third prompt was found where only two were drawn")
	}

	// And nothing is forgotten: the whole screen is what a failure prints,
	// which a buffer cleared between waits could not report.
	if got := drawn.text(); !strings.Contains(got, "echo mark-42") {
		t.Errorf("the screen is %q, want everything drawn kept for the diagnostic", got)
	}
}

// TestAWaitReadsWhatArrivesAfterIt: the cursor must not make a wait miss text
// that had not been drawn when the wait began, which is every real use of it.
func TestAWaitReadsWhatArrivesAfterIt(t *testing.T) {
	pr, pw := io.Pipe()
	t.Cleanup(func() { _ = pw.Close() })
	drawn := watch(t, pr)

	go func() {
		for _, part := range []string{"$ ", "echo mark-42\r\n", "mark-42\r\n", "$ "} {
			time.Sleep(time.Millisecond)
			if _, err := io.WriteString(pw, part); err != nil {
				return
			}
		}
	}()

	drawn.awaitReadyForInput(t)
	drawn.await(t, "mark-42")
	drawn.await(t, "mark-42")
	drawn.awaitReadyForInput(t)
}

// TestAWaitBlocksUntilTheMarkIsDrawnAgain: the cursor is what await is built
// on, so the wait itself is checked and not only the search under it.
//
// The second prompt is arranged to arrive late, and the assertion is on the
// lower bound: a wait satisfied by the prompt it had already seen returns
// before that prompt exists. Load can only make the elapsed time longer,
// which is the direction that keeps this from being a flake of its own.
func TestAWaitBlocksUntilTheMarkIsDrawnAgain(t *testing.T) {
	const late = 50 * time.Millisecond

	drawn := &screen{}
	drawn.buf.WriteString("$ ")
	drawn.awaitReadyForInput(t)

	go func() {
		time.Sleep(late)
		drawn.mu.Lock()
		drawn.buf.WriteString("mark-42\r\n$ ")
		drawn.mu.Unlock()
	}()

	start := time.Now()
	drawn.awaitReadyForInput(t)
	if waited := time.Since(start); waited < late {
		t.Errorf("the wait returned after %v, before the second prompt was drawn at %v", waited, late)
	}
}
