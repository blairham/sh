// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// What this shell can say about the terminal's capabilities, under the
// terminfo and termcap names a script asks for them by.
//
// A **capability** is one thing a terminal can be told to do or one key it
// sends, named by a short word and answered with the bytes that do it: `ed`
// erases to the end of the screen, `kcuu1` is what the Up arrow sends. Two
// name systems are in use for the same set of facts — terminfo's long names
// and termcap's two-letter ones — so each capability here carries both, and
// the value is measured once.
//
// It is here rather than in a dialect package for the reason cellwidth.go is:
// this is a fact about the terminal and about what this shell's own editor
// writes to it, not about which shell is speaking. A dialect owns the *name*
// a script reads it through — one shell calls it `$terminfo` — and nothing
// else about it, which is what keeps `interp` and `syntax` from having to
// know a capability database exists.
//
// # What is answered, and why so little
//
// **This shell does not read the terminfo database.** It answers a fixed set
// of capabilities and refuses every other name — see
// [interp.Runner.SetAbsentElements] for the mechanism and #1388 for what
// happens when a script cannot tell "absent" from "empty".
//
// Two tests, and a capability is answered only when it passes **both**:
//
//  1. **This shell already speaks the sequence.** editor.go writes `\r`,
//     `\e[K`, `\e[J` and `\e[nA`/`B`/`C`/`D` to redraw a line, widgets.go
//     writes `\e[H`, and escape.go's SS3 branch reads `\eOA` through `\eOD`
//     as the arrows. So the value is a fact about this program, checkable by
//     reading the file next to this one.
//  2. **Every terminal description measured spells it the same way.** So the
//     value does not depend on a `$TERM` this table never consults.
//
// Either test alone lets a plausible guess through, and each catches
// something the other does not. `cuu1` — cursor up one line — passes the
// first and fails the second: it is `\e[A` in the xterm family and `\eM`
// under `screen` and `tmux`, so answering it from what the editor happens to
// write would hand a `screen` user an xterm's spelling. `sc` and `rc` — save
// and restore the cursor — pass the second and fail the first: `\e7` and
// `\e8` in every description measured, and nothing in this repository emits
// either, so answering them would be this table making a claim of its own.
// Both refuse.
//
// The measurement behind the second test, 2026-09-07 against zsh 5.9.2's
// `$terminfo` and `$termcap` on this machine's database, over
// `xterm-256color`, `xterm`, `screen`, `screen-256color`, `tmux-256color`,
// `alacritty` and `ghostty`: every value below was byte-identical in all
// seven. The ones that were not are named in terminfoRefused, with what they
// disagreed about, because a reader's first question about a table this short
// is what is missing from it.
//
// # $TERM is not consulted, and that is a choice rather than an omission
//
// Holding it fixed would be a bug if this table were describing a terminal.
// It is describing *this shell*: the editor writes `\e[J` whatever `$TERM`
// says, and interp's `%F{200}` draws the 256-color sequence under `dumb`,
// which is the same choice recorded above interp's colorSequence. So the
// answers here are `$TERM`-independent because the behavior they report is,
// and a table that varied by `$TERM` while the shell did not would be
// describing a terminal this shell declines to adapt to.
//
// Real zsh does adapt, because it reads the database: measured, its
// `$terminfo` is 220 keys under `xterm-256color`, 136 under `screen`, 50
// under `dumb` and **empty** under a `$TERM` its database has never heard of,
// with `${+terminfo}` still 1 in all four. So the fixed table is a real
// difference and not a rounding of one — it is narrower everywhere and
// wider under a `$TERM` nobody has an entry for.
//
// The day a terminfo reader exists this table is where it plugs in, and
// nothing above the seam changes: a dialect asks for a capability by name and
// gets a value or nothing, which is the same question either way.

// TerminalCapability is one capability, under both name systems.
type TerminalCapability struct {
	// Terminfo is the terminfo capability name — the long one.
	Terminfo string

	// Termcap is the termcap capability name — the two-letter one. Every
	// capability here has both, which is a property of the set rather than a
	// guarantee about capabilities in general: termcap named fewer things
	// than terminfo does.
	Termcap string

	// Value is the bytes, or a decimal count for a numeric capability.
	//
	// A string capability whose sequence takes an argument keeps terminfo's
	// own parameter language — `\e[%p1%dA` and not `\e[A` — because that is
	// what the parameter hands a script in the shell being modeled, measured,
	// and a caller either passes it to something that expands it or builds
	// the sequence by hand from the shape.
	Value string
}

// TerminalCapabilities is every capability this shell answers for, in
// terminfo-name order.
//
// A function rather than a package-level table so that no caller can hold a
// reference and change it. Small enough that building it per call costs
// nothing, and every caller wants the whole of it.
func TerminalCapabilities() []TerminalCapability {
	return []TerminalCapability{
		// The one numeric capability, and the only entry whose value is not
		// a sequence. It is read from interp rather than written here so
		// that the count a script is told and the count this shell paints
		// cannot drift apart — see interp.TerminalColors, which is also
		// where the choice not to answer the *terminal's* count is recorded.
		//
		// This is the capability #1388 is about: a plugin manager builds its
		// entire color table behind `-n ${terminfo[colors]}`, so with it
		// absent every message it printed came out as raw markup.
		{Terminfo: "colors", Termcap: "Co", Value: strconv.Itoa(interp.TerminalColors)},
		// The four cursor motions, parameterized. editor.go writes all four
		// while redrawing a wrapped line.
		{Terminfo: "cub", Termcap: "LE", Value: "\x1b[%p1%dD"},
		{Terminfo: "cud", Termcap: "DO", Value: "\x1b[%p1%dB"},
		{Terminfo: "cuf", Termcap: "RI", Value: "\x1b[%p1%dC"},
		{Terminfo: "cuu", Termcap: "UP", Value: "\x1b[%p1%dA"},
		// Carriage return. Written before every redraw, and the one value
		// here that is not an escape sequence at all.
		{Terminfo: "cr", Termcap: "cr", Value: "\r"},
		// Erase to the end of the screen, and to the end of the line.
		// editor.go writes the first to clear a wrapped line it is about to
		// redraw and the second where it knows the line fits on one row.
		{Terminfo: "ed", Termcap: "cd", Value: "\x1b[J"},
		{Terminfo: "el", Termcap: "ce", Value: "\x1b[K"},
		// Cursor to the top left. Written as the first half of the
		// clear-screen widget's `\e[H\e[2J`; the whole of that pair is
		// `clear`, which is refused because the second half disagrees — see
		// terminfoRefused.
		{Terminfo: "home", Termcap: "ho", Value: "\x1b[H"},
		// The four arrow keys, in the application-cursor spelling every
		// description measured gives them. escape.go's SS3 branch reads
		// exactly these four, which is the first test; it also reads the
		// `\e[A` spelling, and a capability answers with one value, so the
		// one the descriptions agree on is the one to give.
		{Terminfo: "kcub1", Termcap: "kl", Value: "\x1bOD"},
		{Terminfo: "kcud1", Termcap: "kd", Value: "\x1bOB"},
		{Terminfo: "kcuf1", Termcap: "kr", Value: "\x1bOC"},
		{Terminfo: "kcuu1", Termcap: "ku", Value: "\x1bOA"},
	}
}

// terminfoRefused is what a reader will look for above and not find, with the
// test each capability failed.
//
// It is a comment with a name rather than prose because the tests read it:
// every name here has to still refuse, so that a capability cannot be added
// to the table above while the sentence explaining its absence stays behind.
// Nothing consults it at run time — a name absent from
// [TerminalCapabilities] refuses by that absence alone, whether it is listed
// here or was never thought about.
//
// The measured disagreements, over the seven descriptions named above:
//
//	cuu1   `\e[A` in the xterm family, `\eM` under screen and tmux
//	clear  `\e[H\e[2J` in the xterm family, `\e[H\e[J` under screen and tmux
//	cnorm  `\e[?12l\e[?25h` in the xterm family, `\e[34h\e[?25h` under screen
//	cvvis  `\e[?12;25h` in the xterm family, `\e[34l` under screen
//	smcup  alacritty appends `\e[22;0;0t` to the xterm sequence, and rmcup
//	rmcup  the matching `\e[23;0;0t`
//	so     `\e[7m` — reverse — in the xterm family, `\e[3m` — italic — under screen
//	se     `\e[27m` against `\e[23m`, the same split
//
// And the ones no description disagreed about, refused for failing the first
// test — nothing in this repository writes or reads them, so a value here
// would be this table's own claim rather than a report of what this shell
// does:
//
//	sc rc     save and restore the cursor: `\e7` and `\e8`
//	civis     hide the cursor: `\e[?25l`. This shell never hides it, and
//	          `cnorm` — showing it again — disagrees anyway, so answering
//	          `civis` alone would offer half of a pair.
//	cud1 cub1 down and back one: `\n` and `\b`. The editor moves by a count
//	cuf1      even when the count is one, so `cuu`/`cud`/`cuf`/`cub` are what
//	          it speaks and the one-step forms are not.
//	ncv       absent from every description measured, real zsh included.
var terminfoRefused = []string{
	"civis", "clear", "cnorm", "cub1", "cud1", "cuf1", "cuu1", "cvvis",
	"ncv", "rc", "rmcup", "sc", "se", "smcup", "so",
}
