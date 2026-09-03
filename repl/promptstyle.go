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

	// Unknown is what happens to an escape whose code is not in Codes. The
	// three shells that have a language give three different answers.
	Unknown UnknownCode

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
	// FieldCwdFull is the path untouched, and FieldCwdBase is its last
	// component alone.
	FieldCwd
	FieldCwdFull
	FieldCwdBase
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
	// The date: bash's `\d` is "Wed Sep 02" and zsh's `%w` is "Wed 2".
	FieldDate
	FieldDateShort
	// FieldEscape is the escape character itself, for the table row that
	// spells it doubled.
	FieldEscape
	// FieldVersion and FieldVersionFull are the version the dialect claims,
	// short and long. bash draws 5.3 for one and 5.3.15 for the other.
	FieldVersion
	FieldVersionFull
)
