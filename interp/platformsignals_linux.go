// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package interp

import "syscall"

// The two named signals this platform has beyond the shared table, and a
// bound thirty-three numbers past where the names stop.
//
// Measured 2026-09-17 in the panel's own Alpine image
// (alpine@sha256:28bd5f…, BusyBox v1.37.0) with Debian-built bash 5.3, dash
// 0.5.12 and zsh 5.9 installed beside it: `kill -l 16` is `STKFLT` in bash,
// zsh and BusyBox ash and `16` in dash, and `kill -l 30` is `PWR` in all
// four. Everything from 1 to 64 is a real send in every column and 65 is
// refused in bash, dash and ash — so the bound is the kernel's range and not
// a list of names, which is what #3168 and #3287 are both about.
//
// The real-time signals in between have names on this platform that this
// table does not carry — bash and dash write `RTMIN+5` for 40 where zsh and
// BusyBox ash write `40`. Numbering them is a separate question from being
// able to send them, and only the second is in range here.
var platformSignals = []signalEntry{
	{"STKFLT", syscall.SIGSTKFLT, true},
	{"PWR", syscall.SIGPWR, true},
}

const platformSignalMax = 64
