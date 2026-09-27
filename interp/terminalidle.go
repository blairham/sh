// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"time"
)

// TerminalIdle is how long it has been since anything was read from the
// terminal this shell holds, and false where it holds none.
//
// The terminal's **access time** is the answer, which is a fact about the
// device file rather than about this program: the kernel stamps it when a
// reader takes characters from the line, so a shell sitting at a prompt with
// nobody typing has a growing one and a shell that has just been typed at has
// a very small one. A shell that keeps its own last-read timestamp would be
// answering about itself, and a shell is not the only thing that can read a
// terminal.
//
// The stream table and not the process's, for the reason
// [Runner.descriptorIsTerminal] gives: an embedded Runner may have been given
// a pipe while the program around it sits at a terminal, and the shell's
// terminal is the shell's. Runner.terminal is the same lookup `$COLUMNS` and
// `read -k` make, remembering included.
//
// False rather than zero where there is no terminal, because zero is a real
// answer — a terminal read from this instant — and the two must not be one
// value. A dialect with a parameter for this writes its own word for the
// absent case.
//
// Negative durations are possible and are handed back as measured: a
// filesystem whose clock is ahead of this one, or a device whose access time
// a debugger has just touched. Deciding what to write for one is the caller's,
// which is the same split as the false above.
func (r *Runner) TerminalIdle() (time.Duration, bool) {
	f, held := r.terminal()
	if !held {
		return 0, false
	}
	return idleSince(f, time.Now())
}

// idleSince is the arithmetic, split out so it can be pointed at an ordinary
// file with a known access time.
//
// That is the whole reason it is a function of its own: a terminal is exactly
// what a test process does not have, so a test of TerminalIdle on a runner
// can only ever see the false branch — which is the shape of a check that
// cannot produce a positive. Pointing this at a file whose access time
// os.Chtimes has just set is a positive the test can watch fire.
func idleSince(f *os.File, now time.Time) (time.Duration, bool) {
	info, err := f.Stat()
	if err != nil {
		return 0, false
	}
	at, ok := fileAccessTime(info)
	if !ok {
		return 0, false
	}
	return now.Sub(at), true
}
