// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"errors"
	"syscall"
)

// The number the last system call this shell made left behind.
//
// One dialect presents it as a parameter and has a builtin that turns it into
// a sentence, and until now there was nothing behind either: `$ERRNO` was an
// ordinary variable, so it read empty however many calls had failed, and
// `syserror` with no operand refused by name (#1802).
//
// **It is not the process's `errno` and must not pretend to be.** This is a Go
// program: the runtime makes system calls of its own on every goroutine, and
// there is no per-shell errno to expose. What it honestly can be is *the error
// the last system call this shell made on a script's behalf saw*, which is the
// only reading a script can act on — and the divergence is written down rather
// than hidden, in Semantics.ErrnoIsTheShellsOwnCalls.
//
// Measured against zsh 5.9.2 on 2026-09-12 with `zmodload zsh/system`:
//
//   - a failed call leaves the number behind and a *successful* one does not
//     clear it, so a stale number outlives the failure that set it;
//   - `ERRNO=13` is an assignment the parameter takes, and `syserror` with no
//     operand then says `Permission denied`;
//   - in a fresh shell `$ERRNO` reads **empty** and `syserror` says
//     `Undefined error: 0` at status 0. The two are not the same state seen
//     twice: the parameter is unset until something assigns it, and the
//     builtin reads the number rather than the parameter.
//
// Only failures are recorded, which is the one place the reading above is
// narrower than the shell's: there, a *successful* libc call overwrites the
// number too, so what a script finds is dominated by calls the shell made for
// its own reasons. Recording only failures is the same "does not reset on
// success" rule with the noise taken out.

// NoteErrno records the number a failed system call left, for the parameter
// and the builtin that present it.
//
// An error that carries no errno leaves the number alone rather than clearing
// it: `os.ErrNotExist` wrapped by something that lost the number is still a
// failure, and replacing a real number with zero would say the opposite of
// what happened.
func (r *Runner) NoteErrno(err error) {
	var errno syscall.Errno
	if err != nil && errors.As(err, &errno) {
		r.lastErrno = int(errno)
	}
}

// LastErrno is that number, and zero when no call this shell made has failed
// — which is a real answer and not an absence: zero is what the sentence
// `Undefined error: 0` names, and it is what the shell being modeled says in
// a fresh session.
func (r *Runner) LastErrno() int { return r.lastErrno }

// SetLastErrno writes it, which is what an assignment to the parameter does.
func (r *Runner) SetLastErrno(n int) { r.lastErrno = n }
