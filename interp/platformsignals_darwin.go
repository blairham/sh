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

// Nothing splits here: the shared table's default actions were written
// against this kernel, SIGIO's discard included, so there is nothing for this
// map to correct. The other platform's copy is where the difference is
// recorded (#3703).
var platformSignalDefaults map[string]bool

// Nothing here is unnamed. The bound is 31 and the table names every number
// from 1 to 31, so the question this constant answers on the other platform —
// what a number in range that has no name does by default — has no case to
// answer here. False is the value that says "no such number", not a measured
// default action (#3777).
const platformUnnamedSignalsEndTheShell = false
