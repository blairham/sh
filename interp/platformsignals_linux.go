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

// platformSignalDefaults are the shared table's entries whose default action
// on this kernel is not the value that table carries.
//
// SIGIO is the one. Its default action here is to **terminate** the process,
// where on the BSD the shared value was written against it is discarded, so
// a shell that sends itself one with nothing trapped stops there.
//
// Measured 2026-09-19, BusyBox v1.37.0 in the digest-pinned alpine image with
// `cmd/ash` cross-compiled for linux/arm64 and run in the same container, each
// probe a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C`:
// `kill -s IO $$` writes `I/O possible` and ends the shell at 157, which is
// 128 + 29, while `trap 'echo hit' IO; kill -s IO $$` prints `hit` and is 0.
//
// It is the only one, and that is measured rather than read off the two
// manuals: every other entry of the shared table whose default action could
// split was sent to the shell in the same container, and URG, CHLD, CONT,
// WINCH, TSTP, TTIN and TTOU are all silent at 0 there while ABRT, SYS, XCPU,
// XFSZ, VTALRM and PROF all end it at 128 + their number — the shared table's
// values for every one of them.
var platformSignalDefaults = map[string]bool{"IO": true}
