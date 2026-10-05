// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package tty

import "syscall"

const tcGets = syscall.TIOCGETA

const tcSets = syscall.TIOCSETA

// vdisable is _POSIX_VDISABLE: the value a special character is set to so that
// no byte is that character. The BSDs spell it 0xff.
const vdisable = 0xff
