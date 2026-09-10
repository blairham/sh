// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import "github.com/blairham/sh/interp"

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
// The ternary `%(x.true.false)` is the one shape that needs a mechanism
// rather than a row in a table — it asks a question about the shell and picks
// one of two texts — so what this table names is which characters open and
// close it and what each test letter asks. See interp/promptconditional.go
// for the construct and #1695 for what it cost: it was the last thing a real
// startup put on standard error, from powerlevel10k's own prompt-length
// routine, which binary-searches on `%$y(l.1.0)`.
//
// `%D`'s braces used to be the second of them. They hold a strftime format,
// which [interp.Strftime] already is, so the code is a Formats entry now:
// measured through a pty, `%D` draws `26-09-07`, `%D{%H:%M}` draws `04:25`
// and `%D{}` draws nothing at all.
//
// Measured through a pty rather than taken from documentation: the panel
// disagrees about this, and the disagreement is why the field exists.
func PromptStyle() interp.PromptStyle {
	return interp.PromptStyle{
		Expand: false,
		// Measured, one code per prompt, through a pty against zsh 5.9.2.
		Escape: '%',
		// A count in front of a code — `%2~` is the last two components. See
		// interp.PromptStyle.NumericArgument and #1592.
		NumericArgument: true,
		Codes: map[rune]interp.PromptField{
			'n': interp.FieldUser,
			'm': interp.FieldHost,
			'M': interp.FieldHostFull,
			'~': interp.FieldCwd,
			'd': interp.FieldCwdFull,
			'/': interp.FieldCwdFull,
			// `%c` and `%.` are the last component of the abbreviated path and
			// `%C` of the unabbreviated one: in the home directory itself the
			// first two drew `~` and the third drew the directory's own name.
			'c': interp.FieldCwdBase,
			'.': interp.FieldCwdBase,
			'C': interp.FieldCwdBaseFull,
			'#': interp.FieldPrivilege,
			'%': interp.FieldEscape,
			't': interp.FieldTime12Padded,
			'@': interp.FieldTime12Padded,
			// zsh's clock does not pad the hour where bash's does: measured at
			// six in the morning, `%*` drew 6:11:43 and `%T` drew 6:11.
			'*': interp.FieldTime24Unpadded,
			'T': interp.FieldTime24HMUnpadded,
			'w': interp.FieldDateShort,
			'W': interp.FieldDateMonthDayYear,
			'D': interp.FieldDateYearMonthDay,
			'?': interp.FieldExitStatus,
			'j': interp.FieldJobCount,
			// Both draw the history number; measured on successive prompts,
			// each drew the one the line about to be typed will have.
			'!': interp.FieldHistoryNumber,
			'h': interp.FieldHistoryNumber,
			'y': interp.FieldTerminalName,
			'_': interp.FieldOpenState,
			// zsh's own non-printing markers. Measured: `%{X%}` drew X and
			// neither marker, and unlike bash's, a marker with no partner is
			// dropped rather than written to the terminal.
			'{': interp.FieldNonPrintingStart,
			'}': interp.FieldNonPrintingEnd,
			// The file being read, which a *script* reaches for far more
			// often than a prompt does: `${(%):-%x}` is the wild idiom for a
			// file's own path. They are rows here because there is one table
			// — this shell answers the same two codes to a drawn prompt,
			// where measured through a pty both come to the shell's own name.
			'x': interp.FieldSourceFile,
			'N': interp.FieldUnitName,
			// Draws nothing and counts as a column, which is what a prompt
			// uses to tell the shell that bytes it has hidden inside `%{ %}`
			// do reach the screen. Measured, `%G` alone leaves the text empty
			// and the column one along, and `%{a%Gb%}` is one column where
			// `%{ab%}` is none.
			'G': interp.FieldCountedColumn,
		},
		// The ternary. Measured letter by letter against zsh 5.9.2, one
		// probe per letter, with the count swept across the range each of
		// them compares against — see interp.PromptCondition, which records
		// what each comparison turned out to be.
		//
		// **A letter absent from here is not a refusal.** Measured, zsh draws
		// nothing at all for `%(a.T.F)` and swallows both arms, at status 0
		// and silently, so a letter that is not below is this language saying
		// it has no such question. The paired half is a letter that *is*
		// below and that a reader cannot answer — `e`, the eval depth, in the
		// interpreter — which is refused by name as `%(e`.
		Conditional:    '(',
		ConditionalEnd: ')',
		Conditions: map[rune]interp.PromptCondition{
			'?': interp.ConditionExitStatus,
			'j': interp.ConditionJobs,
			'#': interp.ConditionEffectiveUser,
			'g': interp.ConditionEffectiveGroup,
			'!': interp.ConditionPrivileged,
			'L': interp.ConditionShellLevel,
			'e': interp.ConditionEvalDepth,
			'_': interp.ConditionOpenConstructs,
			'l': interp.ConditionColumn,
			'S': interp.ConditionSeconds,
			'v': interp.ConditionPromptArrayCount,
			'V': interp.ConditionPromptArrayElement,
			// The working directory, counted whole and counted with the home
			// directory written `~`. Measured in `/Users/bhamilton`, where
			// the first pair answered counts up to two and the second only up
			// to one, because the `~` stands for both of them.
			'/': interp.ConditionCwdComponents,
			'C': interp.ConditionCwdComponents,
			'c': interp.ConditionCwdComponentsHome,
			'.': interp.ConditionCwdComponentsHome,
			'~': interp.ConditionCwdComponentsHome,
			'D': interp.ConditionMonth,
			'd': interp.ConditionDayOfMonth,
			'T': interp.ConditionHour,
			't': interp.ConditionMinute,
			'w': interp.ConditionDayOfWeek,
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
		Colors: map[rune]interp.PromptColor{
			'F': interp.Foreground,
			'K': interp.Background,
		},
		// The one code whose braces are a strftime format rather than a
		// color. See the note above.
		Formats: map[rune]bool{'D': true},
		// `%q` drew nothing at all.
		Unknown: interp.DropBoth,
		// And a `%` with nothing after it is dropped rather than drawn, in
		// both of this shell's readers: measured through a pty, `PS1='x%'`
		// draws `x`, and `print -P 'x%'` and `${(%):-x%}` are `x` too. bash
		// and ksh93 draw the character, which is the zero value.
		TrailingEscapeIsDropped: true,
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
		OpenWords: map[string]interp.OpenWord{
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
			// The pipe that carries standard error has a name of its own
			// here, which is the one place the two spellings of a bar are
			// distinguishable to the person typing: `echo b |&` then a
			// newline draws `errpipe`, where `echo a |` draws `pipe`.
			"|&": {Text: "errpipe"},
			"&&": {Text: "cmdand"},
			"||": {Text: "cmdor"},
		},
		Default:          "%m%# ",
		DefaultContinued: "%_> ",
		// Set and *empty* in a shell with nobody to prompt, which is a third
		// answer rather than either of the other two: measured on `-c` and on
		// a script file alike with nothing inherited, `${PS1+set}` is `set`
		// and `${#PS1}` is 0 — where the three bash members and ksh93 leave
		// the name unset and dash assigns its `$ `. So the values below stay
		// empty on purpose; the flag is what says they were written.
		AssignsWithNobodyToPrompt: true,
	}
}
