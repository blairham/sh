// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package repl

import (
	"os"

	"github.com/blairham/sh/internal/tty"
)

// Raw mode, and where the ioctls went.
//
// The termios plumbing is in internal/tty now, for the reason the terminal
// test moved there ahead of it: `interp` needs the same pair of ioctls —
// `read -k` reads characters from the terminal as they are typed — and `repl`
// imports `interp`, so the dependency only runs one way and a second copy
// here is how the two would come to disagree. What stays is the editor's
// *name* for the thing, because the rest of this package calls it that.
//
// See internal/tty's mode.go, which carries the flag list, why the editor
// wants every one of them off, and the measurement that says `read -k` wants
// only the line buffering.

// terminalState is what was there before raw mode, kept so it can be put back.
type terminalState struct{ mode *tty.Mode }

// makeRaw turns off the line discipline: no echo, no line buffering, no
// signal characters.
func makeRaw(f *os.File) (*terminalState, error) {
	mode, err := tty.Raw(f)
	if err != nil {
		return nil, err
	}
	return &terminalState{mode: mode}, nil
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
	return s.mode.Restore()
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
