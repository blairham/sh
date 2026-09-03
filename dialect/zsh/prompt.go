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
			'_': repl.FieldOpenState,
		},
		// `%q` drew nothing at all.
		Unknown: repl.DropBoth,
		// zsh draws a percent sign where bash draws a dollar.
		Privilege: "%",
		// Measured with nothing assigned: real zsh prompts with the host and
		// the privilege character, and continues with the construct being
		// continued — `for> ` inside a for loop.
		//
		// `%_` draws what the line is still inside, so the continuation says
		// `for> ` inside a for loop the way zsh does.
		// What zsh calls each thing a line can still be inside, measured one
		// construct at a time with PS2='[%_]'. A clause stands in place of
		// the construct it is inside — `if` becomes `then` — and an operator
		// follows it: `true &&` inside a `then` draws `then cmdand`. A
		// loop's `do` is drawn as nothing at all, and the loop stays.
		OpenWords: map[string]repl.OpenWord{
			"for":      {Text: "for"},
			"while":    {Text: "while"},
			"until":    {Text: "until"},
			"select":   {Text: "select"},
			"case":     {Text: "case"},
			"if":       {Text: "if"},
			"then":     {Text: "then", Replaces: true},
			"else":     {Text: "else", Replaces: true},
			"elif":     {Text: "elif", Replaces: true},
			"{":        {Text: "cursh"},
			"function": {Text: "function"},
			"(":        {Text: "subsh"},
			"$(":       {Text: "cmdsubst"},
			"`":        {Text: "bquote"},
			"${":       {Text: "braceparam"},
			"<<":       {Text: "heredoc"},
			"'":        {Text: "quote"},
			`"`:        {Text: "dquote"},
			"|":        {Text: "pipe"},
			"&&":       {Text: "cmdand"},
			"||":       {Text: "cmdor"},
		},
		Default:          "%m%# ",
		DefaultContinued: "%_> ",
	}
}
