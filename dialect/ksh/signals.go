// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh

import "syscall"

// This shell's own words for a signal.
//
// It carries a table rather than asking the machine, and the two disagree
// about more than half of these: `Memory fault` where the host says
// `Segmentation fault`, `Abort` where it says `Abort trap`, `Alarm call` where
// it says `Alarm clock`. Where the words do agree the rendering still does
// not, because this machine's C library writes the number after them and this
// shell never does — so every signal it can report is listed, including the
// ones whose words look the same.
//
// Only the signals that can reach this message are here. The two nothing
// reports are absent, and so are the ones whose default action is to stop or
// to be ignored: a command that was stopped is a job rather than a death.
//
// SIGEMT is absent for the reason knownSignals gives for leaving it out
// there: it is not a signal both platforms have, and naming it would not
// build on the one that does not.
func signalDescriptions() map[syscall.Signal]string {
	return map[syscall.Signal]string{
		syscall.SIGHUP:    "Hangup",
		syscall.SIGQUIT:   "Quit",
		syscall.SIGILL:    "Illegal instruction",
		syscall.SIGTRAP:   "Trace/BPT trap",
		syscall.SIGABRT:   "Abort",
		syscall.SIGFPE:    "Floating exception",
		syscall.SIGKILL:   "Killed",
		syscall.SIGBUS:    "Bus error",
		syscall.SIGSEGV:   "Memory fault",
		syscall.SIGSYS:    "Bad system call",
		syscall.SIGALRM:   "Alarm call",
		syscall.SIGTERM:   "Terminated",
		syscall.SIGXCPU:   "Exceeded CPU time limit",
		syscall.SIGXFSZ:   "Exceeded file size limit",
		syscall.SIGVTALRM: "Virtual time alarm",
		syscall.SIGPROF:   "Profiling time alarm",
		syscall.SIGUSR1:   "User signal 1",
		syscall.SIGUSR2:   "User signal 2",
	}
}
