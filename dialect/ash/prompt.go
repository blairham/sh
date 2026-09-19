// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash

import "github.com/blairham/sh/interp"

// PromptStyle is what ash does to a prompt parameter's value before drawing
// it.
//
// **It has a backslash language, and this file used to say it had none.** The
// sentence was "no escape language beyond what expansion gives", and the
// default `PS1` beside it was the substrate's plain `$ ` — which is what dash
// reports and not what this applet does. Measured 2026-09-18 in the panel's
// own image, `alpine@sha256:28bd5f…`, BusyBox v1.37.0, `env -i
// PATH=/usr/bin:/bin LC_ALL=C HOME=/root TERM=dumb /bin/busybox ash -i` with
// the value assigned on the first line and the second prompt read back
// through `od`: with nothing assigned the prompt in `/` is `/ # `, so the
// default is `\w \$ ` and both codes are drawn (#3570).
//
// The two facts move together. The value alone would have drawn the six
// characters `\w \$ ` at every prompt, which is worse than the `$ ` it
// replaced; the language alone would have left the default saying something
// no shell says.
//
// One probe per prompt, a marker either side of it, and `od` on the whole
// thing — a prompt is written to standard error here, and the applet draws
// one on a pipe as readily as on a terminal, so no pseudo-terminal is needed
// to read it.
//
//	code                    drawn
//	\u                      the user name
//	\h  \H                  the host name to its first dot, and all of it
//	\w  \W                  the directory with $HOME as ~, and its last
//	                        component — nothing at all in `/`
//	\$                      `#` at uid 0 and `$` otherwise
//	\n                      a newline
//	\t  \A  \T  \@          HH:MM, all four alike
//	\e \a \v \f \b \r       ESC, BEL, VT, FF, BS, CR
//	\[  \]                  non-printing markers, both dropped
//	\101 \10 \1             the byte the octal digits name
//	\x41 \x4 \xA1           the byte the hex digits name
//	\x with no digit        `?`
//	\s \d \j \l \V \q \#    the letter alone, backslash dropped
//	\!                      `!`, unchanged over successive prompts
//	a trailing `\`          nothing
//
// Four of those are worth naming because they are **not** bash's, and each
// would read as a table copied across if it were not measured: `\t` is HH:MM
// rather than HH:MM:SS, `\#` and `\!` are the characters themselves rather
// than the command and history numbers, `\W` in `/` is empty rather than `/`,
// and the hexadecimal spelling has no counterpart there at all.
//
// The **doubled** escape is not a row of this table, and the shape that says
// so is worth keeping: `\\B` drew `B` and `\\\\B` drew `\B`. A table row
// mapping `\\` to a backslash would make the first of those `\B`. What
// happens instead is that the expansion runs first — measured directly, since
// `x='\w'; PS1='<<$x>>'` draws the directory, so a code that arrives out of a
// parameter is decoded — and a backslash pair collapses there rather than
// here. So `\\` is simply a code this table does not have, and
// interp.DropEscape produces the backslash the table left behind.
//
// PS2 and PS4 agree with dash and are unchanged; they are what says this is
// about the value and not about the mechanism.
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand:                   interp.PromptExpandsAlways,
		ExpandBeforeEscapes:      true,
		ExpansionSkipsTheEscapes: true,
		Escape:                   '\\',
		Codes: map[rune]interp.PromptField{
			'u': interp.FieldUser,
			'h': interp.FieldHost,
			'H': interp.FieldHostFull,
			'w': interp.FieldCwd,
			'W': interp.FieldCwdBase,
			'$': interp.FieldPrivilege,
			'n': interp.FieldNewline,
			// All four spell the clock the same way here, which is the row
			// that is not bash's: there `\t` is HH:MM:SS and `\T` is the
			// twelve-hour clock.
			't': interp.FieldTime24HM,
			'A': interp.FieldTime24HM,
			'T': interp.FieldTime24HM,
			'@': interp.FieldTime24HM,
			'[': interp.FieldNonPrintingStart,
			']': interp.FieldNonPrintingEnd,
		},
		// The C escapes, each measured as the one byte it names.
		Sequences: map[rune]string{
			'e': "\x1b",
			'a': "\a",
			'v': "\v",
			'f': "\f",
			'b': "\b",
			'r': "\r",
		},
		Octal:                   interp.OctalUpToThree,
		Hex:                     true,
		HexWithNoDigits:         "?",
		Unknown:                 interp.DropEscape,
		TrailingEscapeIsDropped: true,
		Privilege:               "$",
		CwdBaseAtRootIsEmpty:    true,
		// The defaults a script reads out of the parameters, and the ones it
		// draws. `\w \$ ` is this applet's own; PS2 and PS4 are dash's.
		Default:                            `\w \$ `,
		DefaultContinued:                   "> ",
		DefaultTrace:                       "+ ",
		AssignsWithNobodyToPrompt:          true,
		DefaultWithNobodyToPrompt:          `\w \$ `,
		DefaultContinuedWithNobodyToPrompt: "> ",
	}
}
