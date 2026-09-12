// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tty

import (
	"errors"
	"os"
)

// The terminal's line discipline: reading it, changing it, putting it back.
//
// Here rather than in `repl` for the reason IsTerminal is here — `interp` needs
// it too and `repl` imports `interp`, so the dependency only runs one way.
// `read -k` reads characters from the terminal as they are typed, which is the
// same ioctl pair the editor's raw mode begins and ends with; a second copy of
// it beside this one is how the two would come to disagree about which flags
// they clear or whether a restore is idempotent.
//
// **Two modes, because the two callers want different things and the
// difference is measured.** The editor wants everything off: it draws the line
// itself, so the terminal must not echo, must not wait for Return, and must
// not turn ^C into a signal. `read -k` wants only the waiting turned off —
// measured 2026-09-12 against zsh 5.9.2 through a pseudo-terminal, a character
// read by `read -k` **still appears on the screen**, so echo is left exactly
// where it was found. A `read -k` that borrowed the editor's mode would
// silently swallow the keystroke a script asked the person for.

// ErrUnsupported is what every call here answers on a platform with no termios.
var ErrUnsupported = errors.New("tty: the terminal's mode cannot be changed on this platform")

// Mode is a terminal's line discipline as it was before a change, kept so it
// can be put back.
//
// It holds the [os.File] and not a descriptor number: a restore happens later,
// possibly on the way out of a panic, and by then a bare number may have been
// closed and handed out again — see IsTerminal for the same reasoning about
// lifetime.
type Mode struct {
	f     *os.File
	saved modeState
}

// Raw turns the line discipline off: no echo, no line buffering, no signal
// characters, and one byte is enough to return from a read.
//
// What a line editor wants. Echo has to go because the editor draws the line
// itself and is the only thing that knows where the cursor is among the
// characters already there; canonical mode has to go because otherwise nothing
// arrives until Return; and ISIG has to go so that ^C arrives as a byte the
// editor can act on rather than as a signal racing a read already in progress.
func Raw(f *os.File) (*Mode, error) { return setMode(f, true) }

// Cbreak turns off line buffering and leaves the rest — echo included — where
// it was found.
//
// What `read -k` wants: the characters as they are typed, without taking the
// terminal away from whatever else is using it. See the file comment for the
// measurement that says echo stays.
func Cbreak(f *os.File) (*Mode, error) { return setMode(f, false) }

// Restore puts the line discipline back, and is safe on a nil Mode and safe
// twice.
//
// Every path out of a mode change has to reach this, including a panic: a
// shell that leaves echo off makes the terminal unusable, and the next
// keystrokes go nowhere visible.
func (m *Mode) Restore() error {
	if m == nil || m.f == nil {
		return nil
	}
	return putMode(m.f, m.saved)
}

// TranslatesNewlines reports whether this terminal turns a newline into a
// carriage return and a newline on its way out.
//
// The question a reader of a terminal's *output* has to ask before it counts
// columns. Unknown counts as no — a terminal that will not answer is one the
// caller cannot reason about.
func TranslatesNewlines(f *os.File) bool { return postProcessesOutput(f) }

// RawOutput turns off a terminal's output post-processing and nothing else.
//
// Only the output flags. Where this is used the terminal has no reader, so its
// input discipline is nobody's business, and touching more of it than the one
// thing that is wrong would be a second change hidden inside this one — which
// is why it is not spelled as a [Mode]: there is nothing to put back.
func RawOutput(f *os.File) error { return clearOutputPostProcessing(f) }
