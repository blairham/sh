// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash

import (
	"fmt"

	"github.com/blairham/sh/repl"
)

// PromptStyle is what bash does to a prompt parameter's value before drawing
// it.
//
// bash expands the value each time the prompt is drawn — parameters, command
// substitutions and arithmetic alike, so `PS1='$(echo sub)@ '` runs the
// command again at every prompt.
//
// It also has a backslash language of its own, where `\\u` is the user name and
// `\\W` the directory's last component. That is a separate question and is not
// settled here.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{
		Expand: true,
		// Measured, one code per prompt, through a pty. The codes not here are
		// measured too and not yet drawable: the clock (\t \T \A \@ \d), the
		// count of jobs (\j), the history and command numbers (\! \#), the
		// terminal's name (\l) and the version (\v).
		Escape: '\\',
		Codes: map[rune]repl.PromptField{
			'u':  repl.FieldUser,
			'h':  repl.FieldHost,
			'H':  repl.FieldHostFull,
			'w':  repl.FieldCwd,
			'W':  repl.FieldCwdBase,
			's':  repl.FieldShellName,
			'$':  repl.FieldPrivilege,
			'n':  repl.FieldNewline,
			'r':  repl.FieldReturn,
			't':  repl.FieldTime24,
			'T':  repl.FieldTime12,
			'A':  repl.FieldTime24HM,
			'@':  repl.FieldTime12AMPM,
			'd':  repl.FieldDate,
			'v':  repl.FieldVersion,
			'V':  repl.FieldVersionFull,
			'!':  repl.FieldHistoryNumber,
			'#':  repl.FieldCommandNumber,
			'j':  repl.FieldJobCount,
			'l':  repl.FieldTerminalName,
			'\\': repl.FieldEscape,
		},
		// `\q` draws `\q`.
		Unknown:   repl.KeepBoth,
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
