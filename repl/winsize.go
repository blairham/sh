// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"os"

	"github.com/blairham/sh/internal/tty"
)

// terminalSize is how many rows and columns the terminal has, or zeroes where
// it will not say.
//
// A name for internal/tty.Size and not a second implementation of it. The
// ioctl was here first, next to the termios code that needs the neighboring
// call, and it had to move the day `$COLUMNS` became a parameter of the
// language: `interp` cannot reach `repl`, so the shell reading the window from
// the other side of that edge would have had a reader of its own. Two readers
// of one kernel question is the shape this repository has fixed in one copy
// five times — see the note on internal/tty.Size — so there is one, and this
// is the front end's name for it.
//
// Every caller still has to have something sensible to do with zero, which
// means "do not know": a pipe, a closed terminal, or a kernel that declines.
func terminalSize(f *os.File) (rows, cols int) { return tty.Size(f) }

// terminalWidth is the column half, for the editor, which draws in columns and
// has nothing to do with rows.
func terminalWidth(f *os.File) int {
	_, cols := terminalSize(f)
	return cols
}
