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
// user name, `%~` for the directory, `%F{red}` for a color and `%{ %}` around
// anything the terminal is meant to read rather than draw.
//
// Two shapes measured and deliberately absent, both of which need a mechanism
// rather than a row in a table: the braces after `%D`, which hold a strftime
// format, and the ternary `%(x.true.false)`, which asks a question about the
// shell and picks one of two texts. Until they have one, `%D{%F}` draws the
// plain date and then the braces as the text they are, and the ternary is
// dropped the way any code this table does not know is.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() repl.PromptStyle {
	return repl.PromptStyle{
		Expand: false,
		// Measured, one code per prompt, through a pty against zsh 5.9.2.
		Escape: '%',
		Codes: map[rune]repl.PromptField{
			'n': repl.FieldUser,
			'm': repl.FieldHost,
			'M': repl.FieldHostFull,
			'~': repl.FieldCwd,
			'd': repl.FieldCwdFull,
			'/': repl.FieldCwdFull,
			// `%c` and `%.` are the last component of the abbreviated path and
			// `%C` of the unabbreviated one: in the home directory itself the
			// first two drew `~` and the third drew the directory's own name.
			'c': repl.FieldCwdBase,
			'.': repl.FieldCwdBase,
			'C': repl.FieldCwdBaseFull,
			'#': repl.FieldPrivilege,
			'%': repl.FieldEscape,
			't': repl.FieldTime12Padded,
			'@': repl.FieldTime12Padded,
			// zsh's clock does not pad the hour where bash's does: measured at
			// six in the morning, `%*` drew 6:11:43 and `%T` drew 6:11.
			'*': repl.FieldTime24Unpadded,
			'T': repl.FieldTime24HMUnpadded,
			'w': repl.FieldDateShort,
			'W': repl.FieldDateMonthDayYear,
			'D': repl.FieldDateYearMonthDay,
			'?': repl.FieldExitStatus,
			'j': repl.FieldJobCount,
			// Both draw the history number; measured on successive prompts,
			// each drew the one the line about to be typed will have.
			'!': repl.FieldHistoryNumber,
			'h': repl.FieldHistoryNumber,
			'y': repl.FieldTerminalName,
			'_': repl.FieldOpenState,
			// zsh's own non-printing markers. Measured: `%{X%}` drew X and
			// neither marker, and unlike bash's, a marker with no partner is
			// dropped rather than written to the terminal.
			'{': repl.FieldNonPrintingStart,
			'}': repl.FieldNonPrintingEnd,
		},
		// The visual codes, measured one at a time as the exact bytes each put
		// on the wire. `%b` is not bold-off but everything-off — it drew
		// `\e[0m` where `%u` drew `\e[24m` — which is why these are strings
		// the dialect names rather than a notion the substrate has.
		Sequences: map[rune]string{
			'B': "\x1b[1m",
			'b': "\x1b[0m",
			'U': "\x1b[4m",
			'u': "\x1b[24m",
			'S': "\x1b[7m",
			's': "\x1b[27m",
			'f': "\x1b[39m",
			'k': "\x1b[49m",
			'E': "\x1b[K",
		},
		// `%F{red}` drew `\e[31m` and `%K{blue}` drew `\e[44m`.
		Colors: map[rune]repl.PromptColor{
			'F': repl.Foreground,
			'K': repl.Background,
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
