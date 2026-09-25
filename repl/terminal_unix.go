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

// terminalState is what was there before raw mode, kept so it can be put back
// — and whether raw mode is on right now.
//
// The second field is what a session with a line editor it can turn *off* made
// necessary. Raw mode used to be a fact about the whole session: taken once,
// handed back around each command, taken again. A session whose editor is off
// wants the opposite — the terminal in its own discipline for the read as well
// — so the mode moves with the reads, and everything that hands the terminal
// over has to put back the mode the session is actually in rather than raw.
type terminalState struct {
	f    *os.File
	mode *tty.Mode
	raw  bool
}

// terminalFor captures the discipline a session found, and changes nothing.
//
// Nothing rather than raw mode, and that is the whole point of it: a line the
// driver has already written arrives while the discipline is off, is held in
// the raw queue, and is **not** promoted to the canonical queue when canonical
// mode comes back — so a session that takes raw mode at startup and hands it
// back before its first read can lose the line that was already on its way.
// See [tty.Current] for the measurement. Raw mode is taken by the first read
// that wants one.
func terminalFor(f *os.File) (*terminalState, error) {
	mode, err := tty.Current(f)
	if err != nil {
		return nil, err
	}
	return &terminalState{f: f, mode: mode}, nil
}

// makeRaw turns off the line discipline: no echo, no line buffering, no
// signal characters.
//
// It captures the mode it is displacing, so what it hands back is a state of
// its own. That is what a *command* asking the person to edit a line wants —
// see lineread.go — where the session's own state is reached through takeRaw
// below.
func makeRaw(f *os.File) (*terminalState, error) {
	mode, err := tty.Raw(f)
	if err != nil {
		return nil, err
	}
	return &terminalState{f: f, mode: mode, raw: true}, nil
}

// takeRaw turns the line discipline off on a state that already knows what was
// there before, and does nothing where it is off already.
//
// The saved mode is **not** replaced. Capturing again would save whatever is
// there now, which after a restore is the right answer and in raw mode is raw
// — so a second capture is how a session loses the discipline it is supposed
// to hand back.
func (s *terminalState) takeRaw() error {
	if s == nil || s.raw {
		return nil
	}
	if _, err := tty.Raw(s.f); err != nil {
		return err
	}
	s.raw = true
	return nil
}

// isRaw reports whether the line discipline is off right now, which is what
// the newline translation and every handover ask.
func (s *terminalState) isRaw() bool { return s != nil && s.raw }

// restore puts the line discipline back.
//
// Every path out of the editor has to reach this, including a panic: a shell
// that exits leaving echo off makes the terminal unusable, and the user's next
// keystrokes go nowhere visible.
func (s *terminalState) restore() error {
	if s == nil {
		return nil
	}
	s.raw = false
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
