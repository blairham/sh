// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package tty_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/blairham/sh/internal/pty"
	"github.com/blairham/sh/internal/tty"
)

// The character devices that are not terminals, which is the whole reason this
// package exists.
//
// A file-mode test says yes to every one of these — they are all character
// devices — and #525 is what that cost: `read -p` printed its prompt into
// `/dev/random`, and `select` under the ksh93 dialect drew its `#? ` there.
//
// `/dev/null` had a hand-rolled exception before and the other two did not,
// which is why they are here together: the exception is gone, and what
// replaces it is one call that never hears any of these names.
func TestTheCharacterDevicesThatAreNotTerminals(t *testing.T) {
	for _, path := range []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		f, err := os.Open(path)
		if err != nil {
			t.Skipf("no %s: %v", path, err)
		}
		mode, err := f.Stat()
		if err != nil {
			t.Fatal(err)
		}
		// The premise, asserted rather than assumed: if these stopped being
		// character devices the test below would pass for the wrong reason and
		// prove nothing about the difference between the two questions.
		if mode.Mode()&os.ModeCharDevice == 0 {
			t.Errorf("%s is not a character device here, so it is not the case this test is about", path)
		}
		if tty.IsTerminal(f) {
			t.Errorf("%s read as a terminal", path)
		}
		_ = f.Close()
	}
}

// And the things that are not devices at all.
func TestTheOrdinaryStreamsAreNotTerminals(t *testing.T) {
	regular := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(regular, []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(regular)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if tty.IsTerminal(f) {
		t.Error("a regular file read as a terminal")
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if tty.IsTerminal(r) || tty.IsTerminal(w) {
		t.Error("a pipe read as a terminal")
	}

	if tty.IsTerminal(nil) {
		t.Error("nothing at all read as a terminal")
	}
}

// The positive half, which every negative test above needs to mean anything: a
// function that answered false to everything would pass all of them.
func TestARealTerminalIsOne(t *testing.T) {
	control, terminal, err := pty.Open()
	if err != nil {
		t.Skipf("no pseudo-terminal: %v", err)
	}
	defer func() { _ = control.Close() }()
	defer func() { _ = terminal.Close() }()
	if !tty.IsTerminal(terminal) {
		t.Error("a pseudo-terminal's own end did not read as a terminal")
	}
	if !tty.IsTerminal(control) {
		t.Error("the controlling end did not read as a terminal")
	}
}

// Asking does not change the file.
//
// `os.File.Fd` detaches a file from the runtime's poller and leaves it in
// blocking mode for good, which is a real change to make to a pipe merely to
// ask it a question it is going to answer no to. The borrow through SyscallConn
// gives the number back.
//
// Measured on the thing that would break: a pipe read with a deadline, which is
// only possible while the runtime still owns the descriptor.
func TestAskingDoesNotDetachTheFile(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if tty.IsTerminal(r) {
		t.Fatal("a pipe read as a terminal")
	}
	// A deadline is refused outright on a descriptor the poller has given up.
	if err := r.SetReadDeadline(alreadyPast()); err != nil {
		t.Errorf("the pipe will not take a deadline after being asked: %v", err)
	}
	var b [1]byte
	if _, err := r.Read(b[:]); !os.IsTimeout(err) {
		t.Errorf("the read returned %v, want a timeout — the file is no longer polled", err)
	}
}

// alreadyPast is a deadline in the past, so the read below returns at once.
func alreadyPast() time.Time { return time.Now().Add(-time.Second) }
