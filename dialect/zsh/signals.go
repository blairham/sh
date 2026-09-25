// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// This shell's own words for a signal.
//
// A table rather than the machine's, for the reason ksh93 carries one: the two
// disagree. This shell says `abort` where the host says `Abort trap`,
// `invalid system call` where it says `Bad system call`, `alarm` where it says
// `Alarm clock`, and `illegal hardware instruction` where it says `Illegal
// instruction`. Where the words do agree the rendering still does not — every
// word here begins in lower case, and this machine's C library writes the
// signal's number after it where this shell never does — so every signal it
// can report is listed, including the ones that look the same.
//
// Measured 2026-09-25 against `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), on a pseudo-terminal with `TERM=dumb` and the
// shell started `-fiV +Z`: a `sleep 30 &` and one `kill -NAME %%` per row,
// read out of the notice the job got — `[1]  + terminated  sleep 30`.
//
// Only the signals that can reach a notice are here. A signal whose default
// action is to stop or to be ignored never ends a job, and SIGIO is ignored by
// default on this platform, so it has no word to measure and is not guessed
// at. SIGEMT is left out for the reason ksh's table leaves it out: it is not a
// signal both platforms have, and naming it would not build on the one that
// does not — measured, this shell calls it `EMT instruction`.
func signalDescriptions() map[syscall.Signal]string {
	return map[syscall.Signal]string{
		syscall.SIGHUP:    "hangup",
		syscall.SIGINT:    "interrupt",
		syscall.SIGQUIT:   "quit",
		syscall.SIGILL:    "illegal hardware instruction",
		syscall.SIGTRAP:   "trace trap",
		syscall.SIGABRT:   "abort",
		syscall.SIGFPE:    "floating point exception",
		syscall.SIGKILL:   "killed",
		syscall.SIGBUS:    "bus error",
		syscall.SIGSEGV:   "segmentation fault",
		syscall.SIGSYS:    "invalid system call",
		syscall.SIGPIPE:   "broken pipe",
		syscall.SIGALRM:   "alarm",
		syscall.SIGTERM:   "terminated",
		syscall.SIGUSR1:   "user-defined signal 1",
		syscall.SIGUSR2:   "user-defined signal 2",
		syscall.SIGXCPU:   "cpu limit exceeded",
		syscall.SIGXFSZ:   "file size limit exceeded",
		syscall.SIGVTALRM: "virtual time alarm",
		syscall.SIGPROF:   "profile signal",
	}
}
