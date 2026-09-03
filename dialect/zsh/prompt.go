// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/repl"

// PromptStyle is what zsh does to a prompt parameter's value before drawing
// it.
//
// zsh does *not* expand the value. `PS1='<$LOGNAME>@ '` draws `$LOGNAME` as
// it stands, and it takes `setopt PROMPT_SUBST` to change that.
//
// It has its own language instead, spelled with a percent sign — `%n` for the
// user name, `%~` for the directory — which is a separate question and is not
// settled here.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{
		Expand: false,
		// Measured, one code per prompt. The clock codes (%t %* %w) and the full
		// date are measured and not yet drawable.
		Escape: '%',
		Codes: map[rune]repl.PromptField{
			'n': repl.FieldUser,
			'm': repl.FieldHost,
			'M': repl.FieldHostFull,
			'~': repl.FieldCwd,
			'd': repl.FieldCwdFull,
			'C': repl.FieldCwdBase,
			'#': repl.FieldPrivilege,
			'%': repl.FieldEscape,
			't': repl.FieldTime12Padded,
			'*': repl.FieldTime24,
			'w': repl.FieldDateShort,
		},
		// `%q` drew nothing at all.
		Unknown: repl.DropBoth,
		// zsh draws a percent sign where bash draws a dollar.
		Privilege: "%",
		// Measured with nothing assigned: real zsh prompts with the host and
		// the privilege character, and continues with the construct being
		// continued — `for> ` inside a for loop.
		//
		// `%_` is not in the table above, because what it draws is the parser's
		// state and there is no way to ask for that yet. Until there is, an
		// unknown code draws nothing and the continuation comes out as `> `,
		// which is what it was before. The default is written as zsh writes it
		// so that it starts working when the code does.
		Default:          "%m%# ",
		DefaultContinued: "%_> ",
	}
}
