// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// `$COLUMNS` and `$LINES` as parameters of the shell rather than as variables
// something assigns.
//
// The distinction is the whole of this file, and it is what the panel is split
// on. Measured 2026-09-11, all six columns, each through a pseudo-terminal
// opened 100x37 and again on a pipe:
//
//	                  under a pty        no terminal
//	zsh 5.9.2         100 / 37           0 / 0            `-c`, no `-i` needed
//	bash 5.3.15       100 / 37 with -i   unset            interactive only
//	bash-as-sh        100 / 37 with -i   unset            the same build
//	bash 3.2.57       unset even with -i unset            `checkwinsize` is off
//	ksh93             unset              unset
//	dash              unset              unset
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
// # The four answers, in the order they are asked
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
//  4. **Nought**, which is zsh's answer for a shell that has no window and
//     was told nothing. It is a value and not an absence, and the difference
//     is visible: `${COLUMNS-UNSET}` is `0` there and `UNSET` in the five
//     columns without the parameter.
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
	if r.integer == nil {
		r.integer = map[string]bool{}
	}
	if r.integerBase == nil {
		r.integerBase = map[string]int{}
	}
	for _, name := range []string{"COLUMNS", "LINES"} {
		r.integer[name] = true
		// And the base written down, which is what separates a special
		// integer from one a script declared. Measured: `typeset -p COLUMNS`
		// is `typeset -i10 COLUMNS=0` and a plain listing writes `integer 10
		// COLUMNS=0`, where the same shell's `typeset -i x=5; typeset -p x`
		// is `typeset -i x=5` with no base at all. Ten is the base nothing is
		// *written* in, so it marks nothing about the value; it is part of
		// how the name describes itself.
		r.integerBase[name] = 10
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
	rows, cols := r.terminalSize()
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
	// was. Measured: `COLUMNS=55 zsh -f -c 'print $COLUMNS'` is 55.
	if v, ok := r.inheritedValue(name); ok {
		return v
	}
	return "0"
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
