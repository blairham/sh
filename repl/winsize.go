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
//
// A terminal that will not say how wide it is is drawn for at
// [tty.FallbackCols], where the measurement and the reasons live. Zero is not
// a width — it is the ioctl saying it does not know — and taking it literally
// puts the editor on its "nothing known about the terminal" path, where a line
// is assumed to fit on one row and every piece of wrapping arithmetic is
// switched off. That is what every pseudo-terminal fixture in this package was
// grading (#2627).
//
// **The fallback is the drawing's and not the parameter's**, which is the
// split internal/tty declines to make for its callers. There is a screen and
// something has to go on it, so here a guess beats a refusal; `$COLUMNS` has
// somewhere to decline to, and trackWindowSize does decline, because bash on
// exactly this terminal leaves both names unset.
//
// Asked of a terminal rather than of a descriptor. A zero from something that
// is not a terminal at all means there is no screen to guess the width of —
// the distinction internal/tty draws between an unhelpful terminal and the
// absence of one — and the editor is not built for one of those anyway.
func terminalWidth(f *os.File) int {
	_, cols := terminalSize(f)
	if cols <= 0 && tty.IsTerminal(f) {
		return tty.FallbackCols
	}
	return cols
}
