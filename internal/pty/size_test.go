// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package pty

import (
	"errors"
	"syscall"
	"testing"
	"unsafe"
)

// A terminal nobody sized is zero by zero, and a shell asking how wide it is
// gets 0 — a real answer meaning "do not know". A line editor handed that
// draws for a terminal nobody has, so a suite watching it is watching drawing
// that no person would see.
//
// The size is read back through the ioctl that reads it, rather than through
// the struct that was just written, so a field in the wrong place fails here
// rather than showing up as a shell wrapping oddly.
func TestATerminalCanBeGivenASize(t *testing.T) {
	control, terminal, err := Open()
	if errors.Is(err, ErrUnsupported) {
		t.Skip("no pseudo-terminal on this platform")
	}
	if err != nil {
		t.Fatalf("opening a pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})

	if got := read(t, terminal); got.rows != 0 || got.cols != 0 {
		t.Logf("a fresh pair was already %dx%d", got.rows, got.cols)
	}
	const rows, cols = 24, 80
	if err := SetSize(terminal, rows, cols); err != nil {
		t.Fatalf("sizing the terminal: %v", err)
	}
	if got := read(t, terminal); got.rows != rows || got.cols != cols {
		t.Errorf("the terminal is %d rows by %d columns, want %d by %d",
			got.rows, got.cols, rows, cols)
	}
	// Both ends are one terminal, so the size the shell sets is the size the
	// side driving it sees.
	if got := read(t, control); got.rows != rows || got.cols != cols {
		t.Errorf("the control end reports %dx%d, want %dx%d", got.rows, got.cols, rows, cols)
	}
}

func read(t *testing.T, f interface{ Fd() uintptr }) winsize {
	t.Helper()
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	if errno != 0 {
		t.Fatalf("reading the terminal's size: %v", errno)
	}
	return ws
}
