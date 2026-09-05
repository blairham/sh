// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver_test

import (
	"errors"
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
	drawn.await(t, "$ ")
	// Typed as an expression so that the echo of the line and the output of
	// running it are different text: waiting for `mark-42` cannot be
	// satisfied by the shell merely drawing back what was typed.
	write(t, control, "echo mark-$((6 * 7))\r")
	drawn.await(t, "mark-42")
	write(t, control, "\x04") // ^D on an empty line ends the session

	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("status = %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
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

// screen collects everything the shell draws, so a test can wait on a mark in
// it rather than on a duration.
type screen struct {
	mu  sync.Mutex
	buf strings.Builder
}

func watch(t *testing.T, control *os.File) *screen {
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

// await blocks until the mark has been drawn, and fails rather than hanging
// if it never is.
func (s *screen) await(t *testing.T, mark string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(s.text(), mark) {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("waited for %q; drawn so far: %q", mark, s.text())
}
