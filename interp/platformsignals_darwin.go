// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package interp

import "syscall"

// The two signals this platform has and the other does not, and the highest
// number its `kill(2)` will take.
//
// Measured 2026-09-17 on macOS arm64, each reference under a matching
// `argv[0]`: `kill -l 7` is `EMT` and `kill -l 29` is `INFO` in bash 5.3.20,
// zsh 5.9.2 and dash, and `kill -EMT`, `kill -INFO`, `kill -7` and `kill -29`
// all reach a real send in all four columns. ksh93 names EMT and not INFO,
// which is the dialect's own table and not the platform's — see
// Semantics.SignalNamesTheShellLacks.
//
// The bound is 31 because 32 is the first number `kill(2)` answers EINVAL
// for: measured through zsh, the one column that hands an unnamed number
// straight to the kernel, where `kill -31` is a send and `kill -32` is
// `invalid argument`.
var platformSignals = []signalEntry{
	{"EMT", syscall.SIGEMT, true},
	// INFO's default action is to discard the signal, so a shell that has
	// just sent itself one is not dying of it.
	{"INFO", syscall.SIGINFO, false},
}

const platformSignalMax = 31
