// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"fmt"

	"github.com/blairham/sh/interp"
)

// PromptStyle is what bash does to a prompt parameter's value before drawing
// it.
//
// bash expands the value each time the prompt is drawn — parameters, command
// substitutions and arithmetic alike, so `PS1='$(echo sub)@ '` runs the
// command again at every prompt.
//
// It also has a backslash language of its own, where `\\u` is the user name and
// `\\W` the directory's last component, and where `\\[` and `\\]` say that what
// is between them instructs the terminal rather than filling any of it.
//
// One code measured and deliberately absent: `\\D{...}`, whose braces hold a
// strftime format. It needs a translation from that language to Go's layouts
// rather than a row in a table, and until it has one `\\D{%F}` is drawn as it
// was written, which is what an unknown code does here.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand: interp.PromptExpandsAlways,
		// Measured, one code per prompt, through a pty, against bash 5.3.15
		// and bash 3.2.57. The two agree on the whole language: every
		// difference between them was the value of the moment — the clock, the
		// version and the history number — and not the table.
		Escape: '\\',
		Codes: map[rune]interp.PromptField{
			'u':  interp.FieldUser,
			'h':  interp.FieldHost,
			'H':  interp.FieldHostFull,
			'w':  interp.FieldCwd,
			'W':  interp.FieldCwdBase,
			's':  interp.FieldShellName,
			'$':  interp.FieldPrivilege,
			'n':  interp.FieldNewline,
			'r':  interp.FieldReturn,
			't':  interp.FieldTime24,
			'T':  interp.FieldTime12,
			'A':  interp.FieldTime24HM,
			'@':  interp.FieldTime12AMPM,
			'd':  interp.FieldDate,
			'v':  interp.FieldVersion,
			'V':  interp.FieldVersionFull,
			'!':  interp.FieldHistoryNumber,
			'#':  interp.FieldCommandNumber,
			'j':  interp.FieldJobCount,
			'l':  interp.FieldTerminalName,
			'\\': interp.FieldEscape,
			// The two that decide whether a colored prompt is drawn in the
			// right place. Measured: `\[X\]` drew X and neither bracket, and
			// `\[\e]0;title\a\]X` put the title sequence on the wire and drew
			// X — which is the case that cannot be told apart from text by
			// looking at it.
			'[': interp.FieldNonPrintingStart,
			']': interp.FieldNonPrintingEnd,
		},
		// The characters a prompt has no letter for. Measured one byte each:
		// `\e` drew 1b and `\a` drew 07. Without the first, the way every
		// colored bash prompt in the world is written — `\[\e[32m\]` — draws
		// the six characters instead of turning anything green.
		Sequences: map[rune]string{
			'e': "\x1b",
			'a': "\a",
		},
		// `\007` drew the bell and `\101` drew `A`; `\0`, `\1`, `\10` and `\8`
		// were left as written.
		Octal: true,
		// `\q` draws `\q`.
		Unknown:   interp.KeepBoth,
		Privilege: "$",
		// The same numbers the prelude puts in BASH_VERSION. Measured: real
		// bash drew 5.3 for \v and 5.3.15 for \V, and ours claims 5.3.15.
		Version:     fmt.Sprintf("%d.%d", major, minor),
		VersionFull: version,
		// Measured with nothing assigned: real bash prompts `bash-5.3$ `,
		// which is this.
		Default:          `\s-\v\$ `,
		DefaultContinued: "> ",
	}
}
