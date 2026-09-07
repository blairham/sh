// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"io"
	"os"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/pty"
)

// A hook prints with the terminal in its own line discipline.
//
// Raw mode has OPOST off, so a newline written under it is a line feed and
// nothing else and the next thing drawn starts wherever the last one ended.
// The editor holds the terminal in raw mode for the whole of the time between
// two commands — which is exactly when the prompt hook runs — and a hook is a
// shell function whose output goes to the Runner's streams, which nothing in
// this package translates. So the restore is what stands between a one-line
// `precmd` and a prompt twenty columns in.
//
// Asked of the terminal rather than of the bytes, because the bytes only show
// it once something has printed two lines: this is the state the *next* thing
// to print will find, whatever that turns out to be.
func TestWorkHandedTheTerminalBackFindsItsOwnLineDiscipline(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	t.Cleanup(func() {
		_ = terminal.Close()
		_ = control.Close()
	})
	state, err := makeRaw(terminal)
	if err != nil {
		t.Fatalf("raw mode: %v", err)
	}

	s := Shell{In: terminal, Err: io.Discard}
	var during bool
	s.inLineDiscipline(state, func() { during = echoes(t, terminal) })

	if !during {
		t.Error("the work ran with the terminal still in raw mode")
	}
	if echoes(t, terminal) {
		t.Error("raw mode was not put back afterwards")
	}
}

// echoes reports whether the terminal is in its own line discipline, which
// raw mode turns off and restoring turns back on.
func echoes(t *testing.T, f *os.File) bool {
	t.Helper()
	var termios syscall.Termios
	if err := ioctl(int(f.Fd()), tcGets, &termios); err != nil {
		t.Fatalf("reading the terminal: %v", err)
	}
	return termios.Lflag&syscall.ECHO != 0
}
