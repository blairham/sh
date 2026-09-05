// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

// PromptStyle is what a dialect does to a prompt parameter's value before it
// is drawn.
//
// The value is not the prompt. Every shell in the panel transforms it first
// and no two transform it the same way, so the transformation is data a caller
// supplies rather than a rule this package holds — the treatment syntax.Layout
// gets, and for the same reason: a rendering with a shell's taste baked into
// it is a rendering that belongs to that shell.
//
// The zero value draws the value as it stands. That is what a caller without a
// dialect gets, and it is the substrate's own answer rather than a borrowed
// one: told nothing about how to transform the text, it does not transform it.
type PromptStyle struct {
	// Expand runs the value through parameter and command expansion each time
	// the prompt is drawn, so `PS1='$PWD> '` follows the directory and
	// `PS1='$(date +%H:%M) '` follows the clock.
	//
	// Three of the four do this always. zsh does not, unless asked with
	// `setopt PROMPT_SUBST` — which is why this is a field and not a
	// constant, and why it will eventually have to be settable while the
	// shell is running rather than only when it starts.
	Expand bool

	// Escape introduces a code in the prompt: a backslash in bash and ksh93,
	// a percent sign in zsh, and nothing at all in dash. Zero means the
	// dialect has no escape language and the rest of these are never read.
	Escape rune

	// Codes is what each letter after the escape draws. The table is the
	// dialect's, entirely — the same letter means different things in
	// different shells, and most mean nothing in most of them.
	//
	// The escape character doubled belongs in here too, mapped to
	// FieldEscape: all three languages spell "one of these" that way, but it
	// is a row of the table rather than a rule above it.
	Codes map[rune]PromptField

	// Sequences is what a code draws that the substrate has no name for: a
	// fixed string the dialect supplies, written to the terminal as it stands.
	//
	// bash's `\e` and `\a` are two of these — measured, they draw one byte
	// each, 1b and 07 — and so is the whole of zsh's visual language, where
	// `%B` draws `\e[1m` and `%U` draws `\e[4m`. They are here rather than in
	// Codes because there is nothing to compute: the dialect knows what it
	// wants written, and a PromptField for "bold" would be the substrate
	// learning one shell's vocabulary for what is really a terminal's.
	//
	// Read after Codes, so a letter in both is the field.
	Sequences map[rune]string

	// Colors is the codes that set a color, and which half of the screen each
	// sets. The code takes an argument in braces — zsh spells the foreground
	// `%F{red}` — and the sequence itself is the terminal's rather than the
	// dialect's, which is why only the layer is asked for here.
	//
	// Read after Sequences. A code in here with no braces after it takes the
	// empty argument, which is measurable rather than an omission: zsh draws
	// `%F` and `%F{}` alike as `\e[30m`, and `%Fred` as `\e[30m` and then the
	// three letters.
	Colors map[rune]PromptColor

	// Octal says three octal digits after the escape are the byte they name,
	// which is how bash spells a character it has no letter for.
	//
	// Exactly three, measured: `\007` drew the bell and `\101` drew `A`, while
	// `\0`, `\1`, `\10`, `\00` and `\8` were all left as they were written.
	// A value above 255 is taken low byte first — `\400` drew a NUL.
	Octal bool

	// Unknown is what happens to an escape whose code is not in Codes,
	// Sequences or Colors. The three shells that have a language give three
	// different answers.
	Unknown UnknownCode

	// History is a character that draws the history number on its own and
	// itself when doubled. ksh93 spells it `!`, where `<!>` drew 1, 2, 3 on
	// successive prompts and `<!!>` drew `<!>`; bash, dash and zsh draw a
	// bare `!` as a bare `!`.
	//
	// Separate from Codes because it is not a code: nothing introduces it,
	// and it is read in a second pass over what the table has already
	// produced. That order is measurable — ksh93 draws `\!` as the number,
	// which is the backslash being dropped by the table and the `!` left
	// behind being read after.
	History rune

	// Privilege is what FieldPrivilege draws for an ordinary user. bash draws
	// a dollar and zsh draws a percent, which is why the character is here
	// rather than in the mechanism. Root is `#` in both.
	//
	// Empty draws nothing, for a dialect without the notion.
	Privilege string

	// Version and VersionFull are what FieldVersion and FieldVersionFull
	// draw: bash's `\v` is 5.3 and its `\V` is 5.3.15. The strings are the
	// dialect's own, because the version a dialect claims is part of what it
	// claims to be — it is the same number its prelude puts in BASH_VERSION,
	// and the two disagreeing would be a shell lying to one of two questions.
	Version, VersionFull string

	// OpenWords is what this dialect calls each thing the parser can still be
	// inside, for FieldOpenState to draw. A word with no entry is not drawn,
	// which is how one shell says nothing about a loop's `do`.
	OpenWords map[string]OpenWord

	// Default and DefaultContinued are what this dialect prompts with when
	// nothing has been assigned. They are read the same way an assigned value
	// is, so a default may hold codes: bash's is `\s-\v\$ `, which is how
	// it comes to say `bash-5.3$`.
	//
	// Empty means the dialect has not said, and the substrate's own `$ ` and
	// `> ` stand. It does not mean a prompt of nothing — that is a thing only
	// an assignment can ask for.
	Default, DefaultContinued string
}

// OpenWord is what a dialect calls one thing the parser is inside.
type OpenWord struct {
	// Text is the word to draw.
	Text string

	// Replaces says this word stands in place of the one before it rather
	// than following it.
	//
	// Measured: zsh draws `if` as `if` and then, once `then` has been typed,
	// as `then` — the clause instead of the construct. An operator does not
	// do that: `true &&` inside a `then` draws `then cmdand`, both of them.
	// So it is a property of the word and not a rule about clauses.
	Replaces bool
}

// UnknownCode is what becomes of an escape whose code is not in the table.
//
// Measured with a code no shell defines: bash draws `\q` for `\q`, ksh93 draws
// `q`, and zsh draws nothing at all for `%q`. Three shells, three answers, so
// it is asked rather than assumed.
//
// The zero value keeps both characters. That is not bash's answer borrowed: a
// style with no Escape never reaches this question, so the zero value of the
// whole struct has no escape language for it to be about.
type UnknownCode int

const (
	// KeepBoth leaves the escape and its code as they were written, which is
	// what bash does.
	KeepBoth UnknownCode = iota
	// DropEscape keeps the code and drops the escape before it, which is what
	// ksh93 does — and is the whole of ksh93's language, since it has no
	// table behind it at all.
	DropEscape
	// DropBoth removes both, which is what zsh does.
	DropBoth
)

// PromptField is something a prompt draws that is not literal text.
//
// A field rather than a function, so that a dialect's table stays data: a map
// from letters to these can be read, compared and tested, and nothing in a
// dialect package has to know how a shell finds out its own host name.
type PromptField int

const (
	// FieldNone draws nothing. It is the zero value so that a letter missing
	// from a table cannot quietly become something else.
	FieldNone PromptField = iota
	// FieldUser is who the shell is running as.
	FieldUser
	// FieldHost is the host name up to its first dot; FieldHostFull is all of
	// it. bash and zsh both distinguish the two.
	FieldHost
	FieldHostFull
	// FieldCwd is the working directory with the home directory written `~`,
	// FieldCwdFull is the path untouched, FieldCwdBase is the last component
	// of the abbreviated one and FieldCwdBaseFull the last component of the
	// untouched one.
	//
	// The last two differ only at the home directory itself, and there they
	// differ every time a prompt is drawn there: measured in it, bash's `\W`
	// and zsh's `%c` draw `~`, while zsh's `%C` draws the directory's name.
	// Two codes of one shell disagreeing is what says this is two fields.
	FieldCwd
	FieldCwdFull
	FieldCwdBase
	FieldCwdBaseFull
	// FieldPrivilege says whether this is root: `#` when it is, and the
	// dialect's own character when it is not.
	FieldPrivilege
	// FieldShellName is what the shell calls itself.
	FieldShellName
	// FieldNewline, FieldReturn and FieldTab are the characters a prompt
	// cannot hold literally.
	//
	// Note that FieldTab is not what bash's `\t` draws: that is the time.
	// The letters mean what the dialect's table says they mean and nothing
	// carries over from C.
	FieldNewline
	FieldReturn
	FieldTab
	// The clock, in the shapes the panel draws it. Measured rather than
	// chosen: bash's `\t` is 21:55:53, its `\T` is 09:55:56, its `\A` is
	// 21:55 and its `\@` is 09:56 PM; zsh's `%t` is " 9:56PM", padded to two
	// characters of hour, and its `%*` is 21:56:51.
	FieldTime24
	FieldTime12
	FieldTime24HM
	FieldTime12AMPM
	FieldTime12Padded
	// And the same two clocks with the hour not padded at all, which is a
	// difference between the shells rather than between the codes: measured
	// against both at six in the morning, bash's `\t` drew 06:11:40 and zsh's
	// `%*` drew 6:11:43, and bash's `\A` drew 06:11 against zsh's `%T` 6:11.
	FieldTime24Unpadded
	FieldTime24HMUnpadded
	// The date: bash's `\d` is "Wed Sep 02" and zsh's `%w` is "Wed 2".
	FieldDate
	FieldDateShort
	// And zsh's two numeric dates, measured on the sixth of September 2026:
	// `%W` drew 09/06/26 and `%D` drew 26-09-06.
	FieldDateMonthDayYear
	FieldDateYearMonthDay
	// FieldEscape is the escape character itself, for the table row that
	// spells it doubled.
	FieldEscape
	// FieldOpenState is what the shell is still inside, drawn in this
	// dialect's words: `for`, or `for then`, or `quote`. Empty at a prompt
	// that is not a continuation, because nothing is waiting.
	FieldOpenState
	// FieldVersion and FieldVersionFull are the version the dialect claims,
	// short and long. bash draws 5.3 for one and 5.3.15 for the other.
	FieldVersion
	FieldVersionFull
	// FieldHistoryNumber is the number the line about to be typed will have
	// in the history, and FieldCommandNumber is how many commands this
	// session has run. They look alike and are not: measured with three
	// lines already in the history file, bash drew 4 for the first and 1 for
	// the second, because the history carries over between sessions and the
	// count of commands does not.
	FieldHistoryNumber
	FieldCommandNumber
	// FieldJobCount is how many jobs the shell is looking after — 0 before a
	// background command and 1 after it, until it is reaped.
	FieldJobCount
	// FieldTerminalName is the terminal's name without its directory:
	// `ttys013` rather than `/dev/ttys013`.
	FieldTerminalName
	// FieldExitStatus is what the last command exited with, which zsh spells
	// `%?` and bash leaves to `$?` and an expansion.
	FieldExitStatus
	// FieldNonPrintingStart and FieldNonPrintingEnd bracket text that
	// instructs the terminal rather than filling any of it. bash spells them
	// `\[` and `\]`, zsh `%{` and `%}`.
	//
	// They are the most consequential rows in the table and the least visible.
	// Everything the editor does with a line — which row the cursor is on,
	// how far back a redraw has to come, whether the line wrapped at all —
	// counts from how wide the prompt is, and a color is bytes that are not a
	// column. Miscount them and every long line is drawn over itself, which
	// reads to a person as a broken shell rather than as a wrong prompt.
	//
	// Codes rather than a rule above the table for the usual reason: the
	// letters are the dialect's, and a dialect without the notion says
	// nothing and gets nothing.
	FieldNonPrintingStart
	FieldNonPrintingEnd
)

// PromptColor is which half of the screen a color code paints.
//
// The sequence itself is not asked for, because it is the terminal's answer
// rather than the dialect's: see promptcolor.go.
type PromptColor int

const (
	// Foreground is the text's own color, and the zero value so that a code
	// listed without a layer is the common one.
	Foreground PromptColor = iota
	// Background is the color behind it.
	Background
)
