// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux || darwin || freebsd || netbsd || openbsd || dragonfly

package main

import (
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/blairham/sh/internal/smoke"
)

// The terminal's flow control while a line is edited, which zsh leaves to the
// terminal while FLOW_CONTROL is set (#5943).
//
// This editor used to clear `IXON` with the rest of raw mode, so `C-s` and
// `C-q` reached it where in zsh 5.9.2 they stop and start the output. Measured
// 2026-10-05 through a pty, reading the line discipline from the controlling
// side while the shell sat at its prompt:
//
//	defaults                              ixon  -icanon
//	`unsetopt flowcontrol` typed          -ixon at the next prompt
//	`setopt flowcontrol` typed again      ixon
//
// A `stty -ixon` in the startup file is kept, because the editor starts from
// the discipline it found; that is the route a person who wants the keys
// takes in either shell.
//
// `-icanon` is the half that says the reading was taken in the editor's mode
// rather than a command's, so each row also asserts it.
func TestTheEditorLeavesFlowControlToTheTerminalWhileTheOptionIsSet(t *testing.T) {
	control, screen := widgetSession(t)
	atPrompt := func(want bool, what string) {
		t.Helper()
		got, raw := flowAndRaw(t, control)
		if !raw {
			t.Fatalf("%s: the terminal is not in the editor's mode, so this reading says nothing", what)
		}
		if got != want {
			t.Errorf("%s: IXON is %v at the prompt, want %v", what, got, want)
		}
	}
	atPrompt(true, "a fresh session")
	widgetType(t, control, screen, "unsetopt flowcontrol")
	atPrompt(false, "after unsetopt flowcontrol")
	widgetType(t, control, screen, "setopt flowcontrol")
	atPrompt(true, "after setopt flowcontrol")
}

// And what the two settings do to the key. With the option set, `C-s` is the
// terminal's and the editor never hears it — it stops the output, and `C-q`
// starts it again; with it unset, `C-s` is a forward search.
//
// Only `C-s` is sent before the wait, and that is deliberate: a terminal starts
// with IXANY on, and on a terminal that is not a session's controlling one —
// which is what this in-process shell has — any key typed after the stop
// starts the output again. Measured on the pair this test opens; real zsh
// under its own controlling terminal stays stopped until `C-q`.
func TestControlSIsTheTerminalsUntilFlowControlIsUnset(t *testing.T) {
	t.Run("set", func(t *testing.T) {
		control, screen := widgetSession(t)
		if _, err := control.WriteString("\x13"); err != nil {
			t.Fatal(err)
		}
		if !screen.Quiet("i-search", 750*time.Millisecond) {
			t.Fatalf("C-s reached the editor:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
		if _, err := control.WriteString("\x11"); err != nil {
			t.Fatal(err)
		}
		widgetType(t, control, screen, "echo RAN$((40+2))")
		if !strings.Contains(screen.Text(), "RAN42") {
			t.Errorf("the line after C-s C-q did not run:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
	})
	t.Run("unset", func(t *testing.T) {
		control, screen := widgetSession(t, "unsetopt flowcontrol")
		if _, err := control.WriteString("\x13"); err != nil {
			t.Fatal(err)
		}
		if err := screen.Await("fwd-i-search:", widgetBudget); err != nil {
			t.Fatalf("C-s did not start a forward search: %v", err)
		}
		// Out of the search, so the session's own `exit` is read as a line.
		if _, err := control.WriteString("\x07"); err != nil {
			t.Fatal(err)
		}
	})
}

// flowAndRaw reads the terminal's discipline through the controlling side:
// whether XON/XOFF is on, and whether line buffering is off — the editor's
// mode. Read through SyscallConn rather than Fd, which would take the file off
// the poller the screen's reader is using.
func flowAndRaw(t *testing.T, f *os.File) (ixon, raw bool) {
	t.Helper()
	conn, err := f.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var tio syscall.Termios
	var errno syscall.Errno
	if err := conn.Control(func(fd uintptr) {
		_, _, errno = syscall.Syscall6(syscall.SYS_IOCTL, fd, probeTcGets,
			uintptr(unsafe.Pointer(&tio)), 0, 0, 0)
	}); err != nil {
		t.Fatal(err)
	}
	if errno != 0 {
		t.Fatalf("reading the terminal: %v", errno)
	}
	return tio.Iflag&syscall.IXON != 0, tio.Lflag&syscall.ICANON == 0
}
