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
//
// #3535 asked for the names and the answer is that there are none to have.
// `SIGRTMIN` is the *C library's* constant, not the kernel's: the kernel
// gives 32..64 and says nothing about where user-visible numbering starts,
// and each library reserves a different count of the bottom for its own
// threading. Measured 2026-09-19, the same shell answering the same question
// differently on the two libraries:
//
//	kill -l 40     glibc (Debian bookworm)   musl (Alpine)
//	bash 5.3       RTMIN+6                   RTMIN+5
//	dash 0.5.12    RTMIN+6                   RTMIN+5
//
// So RTMIN is 34 under glibc and 35 under musl, and one fixed number has two
// names depending on what the shell was linked against. This shell links
// neither and has no constant to read, so either base would be a guess that
// is wrong on one of the two platforms it runs on. The number is the only
// answer true on both — and it is what zsh and BusyBox ash write in both
// columns. Pinned in platformsignalnames_linux_test.go.
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

// platformUnnamedSignalsEndTheShell is the default action of a number this
// kernel takes that the table above cannot name — the whole of 32 through 64
// here, since 1 through 31 all have names.
//
// It ends the process, and that is a separate fact from the range being real:
// `signalInPlatformRange` made the send succeed (#3287) and the shell then
// outlived it, because the fatality question is asked of a *name* and these
// numbers have none, so every one of them read as survivable. A shell that
// cannot be ended by the signal its supervisor sends it is a wrong answer at
// status 0 (#3777).
//
// Measured 2026-09-19, every number from 32 to 64, each probe a script file
// holding `kill -N $$` then `echo survived` under `env -i PATH=/usr/bin:/bin
// LC_ALL=C` with standard input on /dev/null: BusyBox ash 1.37.0 in the
// digest-pinned alpine image, and bash 5.2.15, dash 0.5.12, zsh 5.9 and ksh93
// in Debian bookworm. All five columns, on both libcs, end at 128 + N for all
// thirty-three numbers and print nothing. Nothing disagrees, so this is core
// rather than an axis — the same reason the range itself was.
//
// The libc split the naming issues turn on does not reach here. Which number
// `RTMIN` is differs between glibc and musl, and a Go binary answers 35 on
// both because the runtime reserves the union of the two reservations — but
// what an *unnamed number* does when it arrives is the kernel's, and the two
// columns above were measured separately and agree number for number.
const platformUnnamedSignalsEndTheShell = true
