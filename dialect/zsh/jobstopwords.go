// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "syscall"

// jobStopWords is what this shell calls a stop, keyed on the signal that made
// it. A signal with no entry takes the plain word, which is what ^Z's SIGTSTP
// gets.
//
// A package-level table rather than a literal inside Diagnostics, because two
// surfaces read it: the listing and the notice, through
// Diagnostics.JobStoppedBySignal, and `$jobstates`, which publishes the same
// word as part of a parameter. Measured 2026-09-26 — see that field for the
// table and for why the word is not the host's name for the signal.
var jobStopWords = map[syscall.Signal]string{
	syscall.SIGSTOP: "suspended (signal)",
	syscall.SIGTTIN: "suspended (tty input)",
	syscall.SIGTTOU: "suspended (tty output)",
}

// jobStateWord is what `$jobstates` calls a job, and what it calls each of the
// processes the job is made of.
//
// One function for both because they are the same word in every shape
// measured: `sleep 9 | cat &` is `running:+:P1=running:P2=running` and a
// stopped job is `suspended:+:P=suspended (signal)`, so the state in front of
// the marker and the state after each `=` are spelled alike.
//
// The two readings that this cannot reach are worth naming rather than
// guessed at. A process whose own state has moved since the shell last looked
// at the job is one of them — `kill -CONT` on a suspended job's pid leaves
// `suspended:+:P=running` there, the job's word stale and the process's
// fresh, which is a second reading of the kernel rather than a second field —
// and a finished job is the other: the table compacts as soon as one ends, so
// a script never sees `done` in it. Neither is reachable here, because this
// shell's job carries one state for the whole of it.
func jobStateWord(stopped bool, stopSig syscall.Signal) string {
	if !stopped {
		return "running"
	}
	if word, ok := jobStopWords[stopSig]; ok {
		return word
	}
	return "suspended"
}
