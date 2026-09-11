// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/tty"
)

// The one place this package asks whether something is a terminal.
//
// It is one place on purpose. The question had two callers by the time `-t`
// needed it — `select`'s menu and `read -p`'s prompt — and a third
// implementation is how this shell has repeatedly ended up with a fixed
// behavior in one construct and the measured one in another. internal/tty is
// where the ioctl lives and why a character-device test is not good enough;
// everything below is descriptor bookkeeping on top of that one call.

// streamIsTerminal reports whether a stream this shell holds is a terminal.
//
// A stream that is not an open file cannot be one: an embedder's buffer, a
// here-document's text, a pipe this package built out of io.Pipe. That is the
// honest answer rather than a guess, and it is the answer a Runner embedded in
// another program gets for the streams it was handed.
//
// shellOwnedFd is deliberately *not* unwrapped, and the reason is that the
// wrapper only ever holds a coprocess pipe — nothing in this package puts a
// terminal behind one — so an unwrapping branch would be a branch no test
// could make answer true, which is worse than not having it.
func streamIsTerminal(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	return tty.IsTerminal(f)
}

// inputIsTerminal reports whether a stream a builtin is about to read is a
// terminal — `select`'s prompt asks about the shell's input, `read -p`'s
// about whatever -u resolved to.
//
// The kernel is asked, through internal/tty, and that is the whole of it. It
// used to be a character-device test with a hand-rolled exception for the null
// device, because the exact answer lived in `repl` and `repl` imports this
// package. #525 is what the approximation cost: every character device that is
// neither a terminal nor `/dev/null` read as a terminal — `/dev/zero`,
// `/dev/random`, a serial port, a printer.
//
// Measured 2026-09-06 against bash 5.3.15, bash 3.2.57 and bash-as-sh, all
// three identical: `read -p 'PROMPT-42 ' v < /dev/random` reads a line, exits
// 0 and prints **no prompt**, where this shell printed one. `/dev/random` is
// the case that says the question is about terminals rather than about the
// null device — the read *succeeds* there, so there is every reason to have
// prompted, and no shell in the panel does.
//
// The null-device exception is gone rather than moved. The ioctl answers
// ENOTTY there without being told the path, so the `os.Stat(os.DevNull)` that
// used to be justified here — a fixed path, outside the boundary, which a
// `-deny /dev/null` policy could have turned into a terminal — is not needed
// by anything any more.
//
// Still a question a Runner may ask: it is about a descriptor the Runner was
// *handed*, not about the process. An embedded Runner may have been given a
// pipe while the program around it sits at a terminal, and this answers about
// the shell's input and not the program's.
func inputIsTerminal(in io.Reader) bool { return streamIsTerminal(in) }

// descriptorIsTerminal answers `-t n`: whether the thing this shell has open
// at one of *its own* descriptor numbers is a terminal.
//
// The shell's table and not the process's, which is the same distinction
// Runner.SystemDescriptor draws and for the same reason. `exec {FD}< file`
// picks a number out of this table; a Runner an embedder built has streams
// that may be nothing the kernel has ever heard of. Asking the process would
// answer about descriptors this shell does not own and would be wrong in
// exactly the session where a library Runner is most likely to be — one
// embedded in a program that does sit at a terminal.
//
// It needs no hook from the front end, and that is the point. The three
// standard streams here *are* what the front end handed in, so a shell binary
// that passes os.Stdin gets the process's terminal without interp ever
// reaching for it, and an embedder that passes a buffer gets false. #1967 is
// what a fixed `false` cost: every `[[ -t 1 ]]` in a startup file took the
// non-terminal arm in a real session, silently.
//
// A number nothing is open at is not a terminal — measured, `[ -t 9 ]` with
// nothing at 9 is false in all six panel columns, on a pipe and on a
// pseudo-terminal alike, and so is a negative number.
func (r *Runner) descriptorIsTerminal(fd int) bool {
	switch fd {
	case 0:
		return streamIsTerminal(r.stdin())
	case 1:
		return streamIsTerminal(r.stdout())
	case 2:
		return streamIsTerminal(r.stderr())
	}
	held, open := r.fds[fd]
	if !open {
		return false
	}
	return streamIsTerminal(held)
}

// terminalTest is `-t` with its operand, shared by `test`, `[` and `[[ ]]`.
//
// Two returns because the operand has to be judged before the descriptor is:
// a word that is not a number is a question the dialects disagree about
// (TerminalTestRequiresANumber), and each construct words its own refusal.
// The answer is false there either way — the shells that stay silent about it
// say false at 1 — so a caller that declines to complain can use it as it is.
//
// The surrounding space is trimmed because four of the six panel columns do:
// `[ -t ' 1 ' ]` on a pseudo-terminal is true in dash, bash 5.3, bash-as-sh,
// bash 3.2 and zsh 5.9.2, and false in ksh93 alone.
func (r *Runner) terminalTest(operand string) (answer, isNumber bool) {
	fd, err := strconv.Atoi(strings.TrimSpace(operand))
	if err != nil {
		return false, false
	}
	return r.descriptorIsTerminal(fd), true
}
