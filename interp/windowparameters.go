// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strconv"
	"strings"

	"github.com/blairham/sh/internal/tty"
)

// `$COLUMNS` and `$LINES` as parameters of the shell rather than as variables
// something assigns.
//
// The distinction is the whole of this file, and it is what the panel is split
// on. Measured 2026-09-11, all six columns, each through a pseudo-terminal
// opened 100x37 and again on a pipe; the middle column added 2026-09-13, on a
// pseudo-terminal opened with *no* window size, which answers the ioctl 0x0:
//
//	                  under a pty        a pty of no size   no terminal
//	zsh 5.9.2         100 / 37           80 / 24            0 / 0
//	bash 5.3.15       100 / 37 with -i   unset              unset
//	bash-as-sh        100 / 37 with -i   unset              the same build
//	bash 3.2.57       unset even with -i unset              `checkwinsize` off
//	ksh93             unset              unset              unset
//	dash              unset              unset              unset
//
// zsh needs no `-i` for any of it, which is the half that separates its
// reading from bash's.
//
// So the *variable* half of this is already answered elsewhere and is bash's:
// Runner.TracksWindowSize is the permission, repl.trackWindowSize does the
// assigning, and it happens once per prompt because that is what `checkwinsize`
// promises. Nothing below changes it.
//
// What is here is the other reading, and only zsh has it. There the two names
// are the shell's own — produced on being read, present with no terminal and
// no prompt anywhere in sight, and typed:
//
//	zsh -f -c 'print -r -- "${(t)COLUMNS}"'      integer-special
//	zsh -f -c 'COLUMNS="3+4"; print $COLUMNS'    7        an expression
//	zsh -f -c 'COLUMNS=abc;   print $COLUMNS'    0        and not text
//	zsh -f -c 'typeset -p COLUMNS'               typeset -i10 COLUMNS=0
//
// which is why registering them is not `SetVar` with better timing. A stored
// string cannot be `integer-special`, cannot be 0 where there is no window,
// and cannot follow a window that changes while a script runs.
//
// # The five answers, in the order they are asked
//
// Each of the two names answers from the first of these that has something to
// say, and the order is measured rather than chosen:
//
//  1. **What a script assigned**, until the window changes under it. See
//     windowSizeValue — this is the one a reader is most likely to get wrong.
//  2. **The terminal**, which is what makes the pair worth having.
//  3. **The environment**, for a shell with no terminal that inherited one:
//     `COLUMNS=55 zsh -f -c 'print $COLUMNS'` is 55 and its type is
//     `integer-export-special`, so the inherited value is kept and stays
//     exported. With a terminal the terminal wins — the same line under a
//     100-column pty is 100.
//  4. **The classic 80 by 24**, where there is a terminal and it will not say
//     how big it is. A pseudo-terminal created without a window size answers
//     the ioctl with 0x0 — `pty.fork()` does exactly that — and zsh reports
//     `COLUMNS=80 LINES=24` there, with `typeset -p COLUMNS` writing
//     `typeset -i10 COLUMNS=80`. Measured 2026-09-13.
//  5. **Nought**, which is zsh's answer for a shell that has no window at all
//     and was told nothing. It is a value and not an absence, and the
//     difference is visible: `${COLUMNS-UNSET}` is `0` there and `UNSET` in
//     the five columns without the parameter.
//
// Four and five are the two readings of a zero and they are not the same
// answer: on a pipe the same shell reports 0. Taking the ioctl's 0 literally
// is what silently disabled every piece of wrapping arithmetic (#2489).
//
// # What `unset` does
//
// Takes the name away entirely, and it stays away — `${(t)COLUMNS}` is empty
// afterwards, and measured through a resize, a window that changes does *not*
// bring it back. Nothing here has to arrange that: `unset` records the removal
// and a removed name is never asked of its producer. Assigning the name again
// restores the parameter, type and all, which is the same path every produced
// parameter takes back.

// ProvideWindowSize installs `$COLUMNS` and `$LINES` as this shell's own
// integer parameters, following the terminal.
//
// For the dialect whose shell has them that way, which is zsh alone. It is in
// this package rather than in that one for the reason every other capability
// here is: the names are bash's too, the question "how wide is the terminal"
// has exactly one right answer and one reader (internal/tty.Size), and a
// dialect that implemented it would be implementing it for the next dialect
// that asks — or, far more likely, instead of it.
//
// The integer attribute is set as well as the producer registered, and it is
// not decoration. It is what makes `COLUMNS="3+4"` seven and `COLUMNS=abc`
// nought, which is the arithmetic-on-assignment every integer name here
// already does, and it is what `${(t)COLUMNS}` reads to say `integer` rather
// than `scalar`.
func (r *Runner) ProvideWindowSize() {
	r.providesWindowSize = true
	for _, name := range []string{"COLUMNS", "LINES"} {
		// With the base written down, which is what separates a special
		// integer from one a script declared. Measured: `typeset -p COLUMNS`
		// is `typeset -i10 COLUMNS=0` and a plain listing writes `integer 10
		// COLUMNS=0`, where the same shell's `typeset -i x=5; typeset -p x`
		// is `typeset -i x=5` with no base at all. See markIntegerParameter,
		// which is the one place the two tables are written.
		r.markIntegerParameter(name, 10)
	}
	r.SetDynamic("COLUMNS", func(r *Runner) string { return r.windowSizeValue("COLUMNS") })
	r.SetDynamic("LINES", func(r *Runner) string { return r.windowSizeValue("LINES") })
}

// windowSizeValue is one of the two names, read.
//
// # A window that changes supersedes what a script assigned
//
// This is the rule a reader gets wrong, and it is wrong silently: every static
// probe passes and the shell is only ever wrong after somebody drags a corner.
// Measured through a pseudo-terminal resized from 80x24 to 120x40 while the
// script was in a `sleep`, zsh 5.9.2 under plain `-c`:
//
//	print $COLUMNS; COLUMNS=13; print $COLUMNS; print $COLUMNS   80 13 13
//	…then the window is resized…                 print $COLUMNS  120
//
// So neither "the assignment wins" nor "the terminal wins" is the rule: the
// assignment stands until the window moves, and the window moving takes it
// away. It is not a signal handler either — the same measurement with
// `trap "" WINCH` in front of it still reports 120, so the value is derived
// from the terminal rather than pushed into the parameter by SIGWINCH, which
// is why reading it here answers the question without this package owning a
// signal.
//
// Both names are superseded together, because one resize moves both numbers
// and a script that assigned only `COLUMNS` still had `LINES` change under it.
//
// The size is *settled* on the first read rather than at registration, and the
// difference is a whole class of bug: a dialect applies itself before a front
// end has finished handing the Runner its streams, so a size recorded at
// registration is the size of no terminal, and the first read would then see a
// change that never happened and throw away an assignment the script had
// already made. `COLUMNS=13; print $COLUMNS` is 13 in zsh, and settling late
// is what makes it 13 here.
func (r *Runner) windowSizeValue(name string) string {
	rows, cols, held := r.terminalSize()
	switch {
	case !r.windowSettled:
		r.windowSettled = true
		r.windowRows, r.windowCols = rows, cols
	case rows != r.windowRows || cols != r.windowCols:
		r.windowRows, r.windowCols = rows, cols
		delete(r.assigned, "COLUMNS")
		delete(r.assigned, "LINES")
	}
	if v, ok := r.assigned[name]; ok {
		return v
	}
	size := cols
	if name == "LINES" {
		size = rows
	}
	if size > 0 {
		return itoa(size)
	}
	// A shell with no window keeps what it was handed, which is how a script
	// run from one that had a terminal still knows how wide that terminal
	// was. Measured: `COLUMNS=55 zsh -f -c 'print $COLUMNS'` is 55, and it is
	// still 55 under a terminal that will not say its size, so the
	// environment stands *in front of* the fallback below and not behind it.
	if v, ok := r.inheritedValue(name); ok {
		return v
	}
	// A terminal that is there and will not say how big it is, which is the
	// fifth answer and is not the fourth. Measured 2026-09-13 on a
	// pseudo-terminal created without a window size — what `pty.fork()`
	// produces, and the window between `forkpty` and the parent's
	// `TIOCSWINSZ` — zsh 5.9.2 reports `COLUMNS=80 LINES=24` where the ioctl
	// says 0x0, and `typeset -p COLUMNS` writes `typeset -i10 COLUMNS=80`. On
	// a pipe the same shell reports 0, so the fallback is about an unhelpful
	// terminal rather than about the absence of one.
	//
	// Nothing else in the panel has an opinion here, because nothing else has
	// the parameter: bash under `-i` on the same 0x0 pseudo-terminal leaves
	// both names *unset*, which is what its `checkwinsize` does at every size
	// it cannot read.
	if held {
		if name == "LINES" {
			return itoa(tty.FallbackRows)
		}
		return itoa(tty.FallbackCols)
	}
	return "0"
}

// ScreenSize is how big the terminal is, answered the way a *terminal
// capability* asks it rather than the way `$COLUMNS` does.
//
// Exported for the dialect that presents the terminal's description as a
// parameter: `$terminfo[cols]` and `$terminfo[lines]` are the screen's size
// there and not the description's, so the two names have to be answered from
// the same reader `$COLUMNS` uses. It is here rather than in that dialect for
// the reason ProvideWindowSize gives — the question has one right answer and
// one reader, and a dialect that implemented it would be implementing it for
// the next one to ask.
//
// **Three answers where windowSizeValue has five**, and the two it does not
// have are the point of the separate entry. Measured against zsh 5.9.2
// through a pseudo-terminal opened 100x37, 2026-09-14:
//
//	pty 100x37, TERM=xterm-256color   cols=100 lines=37   the terminal,
//	                                  not the description's 80x24
//	pty 100x37, COLUMNS=55 in env     cols=100 lines=37   the terminal wins
//	pty 100x37, COLUMNS=77 assigned   cols=100 lines=37   and an assignment
//	                                  moves `$COLUMNS` and not this
//	pty of no size                    cols=80  lines=24
//	no terminal, COLUMNS=100 in env   cols=100 lines=40
//	no terminal, nothing in the env   cols=80  lines=24   where `$COLUMNS`
//	                                  is 0
//
// So what a script assigned is **not** consulted — row three — and a shell
// with no window at all answers the classic 80 by 24 rather than the nought
// `${COLUMNS}` reports there. Both differences are why this is not
// windowSizeValue with a different caller.
func (r *Runner) ScreenSize() (rows, cols int) {
	rows, cols, _ = r.terminalSize()
	if cols <= 0 {
		cols = r.inheritedSize("COLUMNS", tty.FallbackCols)
	}
	if rows <= 0 {
		rows = r.inheritedSize("LINES", tty.FallbackRows)
	}
	return rows, cols
}

// inheritedSize is the environment's answer for one of the two names, and the
// classic fallback where it has none or where what it has is not a count.
func (r *Runner) inheritedSize(name string, fallback int) int {
	v, ok := r.inheritedValue(name)
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// isWindowSizeParameter reports whether this name is one of the two the shell
// itself provides.
//
// It is asked where an *attribute* is about to be taken off a name, because
// the attributes of a parameter the shell is are not a script's to lose — see
// clearAttributes. Guarded on the permission as well as on the two spellings:
// in a shell without the capability `COLUMNS` is an ordinary variable somebody
// assigned, and an ordinary variable loses its letters to `unset` like any
// other.
func (r *Runner) isWindowSizeParameter(name string) bool {
	return r.providesWindowSize && (name == "COLUMNS" || name == "LINES")
}
