// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/blairham/sh/internal/tty"
)

// Raw mode, through syscall directly.
//
// No dependency for it: this module has none outside the linter's tooling, and
// a shell reaching for a terminal library to turn off echo would be a poor
// trade. Two ioctls and a struct is the whole of it.

// terminalState is what was there before raw mode, kept so it can be put back.
type terminalState struct {
	fd    int
	saved syscall.Termios
}

// makeRaw turns off the line discipline: no echo, no line buffering, no
// signal characters.
//
// The shell wants the bytes as they are typed. Echo has to go because the
// editor draws the line itself — it is the only one that knows where the
// cursor is among the characters already there. Canonical mode has to go
// because otherwise nothing arrives until Return. And ISIG has to go so that
// ^C arrives as a byte the editor can act on rather than as a signal that
// would have to be caught and raced against a read already in progress.
func makeRaw(f *os.File) (*terminalState, error) {
	fd := int(f.Fd())
	var t syscall.Termios
	if err := ioctl(fd, tcGets, &t); err != nil {
		return nil, err
	}
	saved := t
	t.Iflag &^= syscall.IGNBRK | syscall.BRKINT | syscall.PARMRK | syscall.ISTRIP |
		syscall.INLCR | syscall.IGNCR | syscall.ICRNL | syscall.IXON
	t.Oflag &^= syscall.OPOST
	t.Lflag &^= syscall.ECHO | syscall.ECHONL | syscall.ICANON | syscall.ISIG | syscall.IEXTEN
	t.Cflag &^= syscall.CSIZE | syscall.PARENB
	t.Cflag |= syscall.CS8
	// One byte is enough to return from a read, and no timer: the editor
	// blocks until something is typed rather than spinning.
	t.Cc[syscall.VMIN] = 1
	t.Cc[syscall.VTIME] = 0
	if err := ioctl(fd, tcSets, &t); err != nil {
		return nil, err
	}
	return &terminalState{fd: fd, saved: saved}, nil
}

// restore puts the line discipline back.
//
// Every path out of the editor has to reach this, including a panic: a shell
// that exits leaving echo off makes the terminal unusable, and the user's next
// keystrokes go nowhere visible.
func (s *terminalState) restore() error {
	if s == nil {
		return nil
	}
	saved := s.saved
	return ioctl(s.fd, tcSets, &saved)
}

func ioctl(fd int, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, uintptr(fd), req,
		uintptr(unsafe.Pointer(t)), 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// IsTerminal reports whether this file is one.
//
// The question is put to the kernel as an ioctl — the same one raw mode begins
// with — because that is the only test that answers it. A character device is
// a strictly weaker question, and `/dev/null`, `/dev/zero` and `/dev/random`
// all pass it while being nobody's terminal. See internal/tty, which holds the
// call and the measurements.
//
// Asked of the file rather than of the process, because the shell's input is
// the question and not the program's: a Runner embedded in something else may
// have been handed a pipe while the program around it sits at a terminal.
//
// **The implementation moved and this name did not.** `interp` has to ask the
// same question — `select`'s prompt and `read -p`'s (#525) — and `repl` imports
// `interp`, so the dependency only runs one way and the ioctl could not stay
// here. It is in internal/tty now, which both import. This stays exported
// because the front end asks it by this name: `driver` decides whether to
// prompt, and `cmd/sh` whether a connection has a person on it. The decision
// stays in `driver`; the answer is one implementation, which is the point.
func IsTerminal(f *os.File) bool { return tty.IsTerminal(f) }
