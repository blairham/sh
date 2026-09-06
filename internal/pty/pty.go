// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package pty opens a pseudo-terminal pair, so that a test can hand a shell a
// real terminal.
//
// It exists because the interesting half of "is this a terminal" cannot be
// tested any other way. A pipe, a file and the null device all prove the
// negative; nothing short of a terminal proves that a shell handed one
// prompts, and a test process has no terminal it can rely on — `go test`
// under CI has none at all, and one it inherited from a developer's shell is
// not a thing to write assertions against.
//
// It began as test infrastructure and is no longer only that. `repl` opens a
// pair in a running shell — the conduit that lets output be captured without
// a child losing its terminal (#720) — so "nothing outside a test builds a
// terminal" is no longer true and is not left standing here. Still internal,
// because it is this module's own and not an interface anybody outside it
// should be holding.
//
// There is no portable call for this outside libc and this module does not
// use cgo, so each family is opened its own way — see the per-platform files.
// A platform with neither returns [ErrUnsupported] and the tests skip.
package pty

import (
	"errors"
	"os"
)

// ErrUnsupported is returned where this package does not know how to open a
// pseudo-terminal. A test should skip rather than fail: a terminal it cannot
// make is not evidence about the shell.
var ErrUnsupported = errors.New("pty: no pseudo-terminal on this platform")

// Open returns the two ends of a new pseudo-terminal.
//
// Writing to control puts bytes where a shell reading terminal sees
// keystrokes; reading control collects what the shell drew. Both are the
// caller's to close, and closing control is what gives the shell an end of
// input.
func Open() (control, terminal *os.File, err error) { return open() }
