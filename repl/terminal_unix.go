// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"
	"syscall"
	"unsafe"
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

// isTerminal reports whether this file is one.
//
// The same test interp uses for `select`: a character device, asked of the
// file rather than of the process, because the shell's input is the question
// and not the program's.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
