// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package tty_test

import (
	"syscall"
	"testing"
	"unsafe"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/tty"
)

// The two modes, and the one difference between them that matters.
//
// A test that read the terminal back through the package that wrote it would
// pass whenever the two halves agreed, including when both were wrong about
// which flag they touch. So these ask the kernel directly — the same reason
// internal/acpcheck writes the wire format out again by hand rather than
// importing the package it grades.

func probe(t *testing.T, fd uintptr) syscall.Termios {
	t.Helper()
	var s syscall.Termios
	_, _, errno := syscall.Syscall6(syscall.SYS_IOCTL, fd, probeGets,
		uintptr(unsafe.Pointer(&s)), 0, 0, 0)
	if errno != 0 {
		t.Fatalf("reading the terminal: %v", errno)
	}
	return s
}

// Cbreak turns off the waiting and leaves echo exactly where it was.
//
// This is the whole reason there are two modes rather than one. Measured
// 2026-09-12 against zsh 5.9.2 through a pseudo-terminal: a character read by
// `read -k` still appears on the screen. A `read -k` built on the editor's raw
// mode would silently swallow the keystroke a script just asked a person for
// — and would look correct in every test that only checked what was read.
func TestCbreakStopsTheWaitingAndLeavesEchoAlone(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })

	before := probe(t, terminal.Fd())
	if before.Lflag&syscall.ECHO == 0 {
		t.Skip("this pseudo-terminal does not start with echo on")
	}
	mode, err := tty.Cbreak(terminal)
	if err != nil {
		t.Fatal(err)
	}
	during := probe(t, terminal.Fd())
	if during.Lflag&syscall.ICANON != 0 {
		t.Error("cbreak left the line buffering on, so nothing arrives until Return")
	}
	if during.Lflag&syscall.ECHO == 0 {
		t.Error("cbreak turned echo off, so a keystroke a script asked for would vanish")
	}
	if during.Lflag&syscall.ISIG == 0 {
		t.Error("cbreak took the signal characters, which is the editor's business and not this one's")
	}
	if err := mode.Restore(); err != nil {
		t.Fatal(err)
	}
	if after := probe(t, terminal.Fd()); after.Lflag != before.Lflag {
		t.Errorf("Lflag %#x after the restore, want %#x", after.Lflag, before.Lflag)
	}
}

// Raw is the other one: everything off, which is what a line editor needs
// because it draws the line itself.
func TestRawTakesEchoAndTheSignalCharacters(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })

	before := probe(t, terminal.Fd())
	mode, err := tty.Raw(terminal)
	if err != nil {
		t.Fatal(err)
	}
	during := probe(t, terminal.Fd())
	// uint64 on both sides: the flag word is 64 bits on the BSDs and 32 on
	// Linux, so the constant is widened rather than the field assumed.
	for _, f := range []struct {
		name string
		bit  uint64
	}{
		{"ECHO", syscall.ECHO},
		{"ICANON", syscall.ICANON},
		{"ISIG", syscall.ISIG},
		{"IEXTEN", syscall.IEXTEN},
	} {
		if uint64(during.Lflag)&f.bit != 0 {
			t.Errorf("raw mode left %s on", f.name)
		}
	}
	if during.Oflag&syscall.OPOST != 0 {
		t.Error("raw mode left the output post-processing on")
	}
	if err := mode.Restore(); err != nil {
		t.Fatal(err)
	}
	after := probe(t, terminal.Fd())
	if after.Lflag != before.Lflag || after.Oflag != before.Oflag {
		t.Error("the restore did not put the discipline back")
	}
}

// Restoring is safe on nothing and safe twice, because the callers defer it
// and one of them may have restored already on its way out.
func TestRestoringIsSafeOnNothingAndTwice(t *testing.T) {
	var none *tty.Mode
	if err := none.Restore(); err != nil {
		t.Errorf("restoring nothing: %v", err)
	}
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() { _ = control.Close(); _ = terminal.Close() })
	mode, err := tty.Cbreak(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := mode.Restore(); err != nil {
		t.Fatal(err)
	}
	if err := mode.Restore(); err != nil {
		t.Errorf("restoring twice: %v", err)
	}
}

// Nothing that is not a terminal takes a mode, and a nil file is not a panic.
func TestOnlyATerminalTakesAMode(t *testing.T) {
	if _, err := tty.Cbreak(nil); err == nil {
		t.Error("a nil file took a mode")
	}
	if _, err := tty.Raw(nil); err == nil {
		t.Error("a nil file took a mode")
	}
	if err := tty.RawOutput(nil); err == nil {
		t.Error("a nil file took the output mode")
	}
	if tty.TranslatesNewlines(nil) {
		t.Error("a nil file said it translates newlines")
	}
}
