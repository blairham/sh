// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// The prompt-escape language: one table, and the two readers that read it.
//
// A shell answers the same question twice. A **prompt** is drawn from it, and
// a **script** asks for the same expansion by name — `${(%)…}` and its other
// spelling `print -P`, which #1091 established are one expansion rather than
// two. Both are the escape language, so both must be the same table.
//
// They were not. `interp` carried four escapes and refused everything else by
// name; `repl.PromptStyle` carried about forty, filled in by the dialect, and
// the prompt drawer read that one. So the shell answered `%F{196}` when
// drawing a prompt and refused it when a script wrote
// `print -P '%F{196}…'` — one question, two answers, and the larger table was
// the one a script could not reach (#1090).
//
// The table lives here, in the package both readers can see, and it is
// supplied by a dialect the way [Semantics] and [Diagnostics] are —
// [Runner.SetPromptStyle]. `repl.PromptStyle` is an *alias* for this type
// rather than a copy of it, so there is one table by construction and not by
// upkeep.
//
// **What is still two is the resolver, and that is the point.** A code's
// *value* can differ between the two readers, because a drawer knows things a
// script's expansion does not: which line of the session's history is about
// to be typed, what the terminal is called, what construct the parser is
// still inside. Those are facts about a session and not about a shell, so
// this package answers what a Runner holds, refuses the rest **by name**, and
// leaves the session's answers to the drawer. Two codes differ deliberately
// and are measured either way: `%{` and `%}` are nothing at all to a script
// and are the drawer's width markers in a prompt, and a newline is `\n` to a
// script where a prompt drawn in raw mode needs the carriage return with it.
//
// Everything above the resolver — which letters exist, what a bare escape
// does, where a color argument ends, what an unknown code becomes — is this
// one table and this one walker.

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
// PromptExpandsAlways is [PromptStyle.Expand] for a shell that expands a
// prompt unconditionally, which is three of the four in the panel.
func PromptExpandsAlways(*Runner) bool { return true }

type PromptStyle struct {
	// Expand says whether the value goes through parameter and command
	// expansion each time the prompt is drawn, so `PS1='$PWD> '` follows the
	// directory and `PS1='$(date +%H:%M) '` follows the clock.
	//
	// **Asked at every draw rather than read once**, and that is the whole
	// reason it is a function. Three of the four shells expand always, and
	// the fourth lets a *running script* change the answer: `setopt
	// PROMPT_SUBST` turns it on and `unsetopt` turns it back off, so a value
	// settled when the shell started is the wrong answer for every prompt
	// after the one that moved it.
	//
	// It cost a whole prompt theme to leave as a constant. A theme's entire
	// `PROMPT` is `${…}` that only means anything expanded, it sets the
	// option in its own setup, and this shell — answering no, forever, from
	// a value read at startup — drew the raw text of the parameter at every
	// prompt, with nothing reported.
	//
	// nil is never, which is the zero value and what a caller with no
	// dialect gets. [PromptExpandsAlways] is the other constant answer.
	Expand func(*Runner) bool

	// ExpandBeforeEscapes runs the expansion *first* and lets the escape
	// table read what it produced, instead of expanding what the table
	// already rewrote.
	//
	// A real disagreement rather than an ordering nobody thought about, and
	// both halves are measured. In bash, with `C='\u'` and
	// `PS1='A${C}B\u C '`, the prompt draws `A\uBbhamilton C` — the `\u`
	// that came out of the parameter is **not** decoded, so the escapes are
	// read before the expansion runs. In zsh, with `C='%F{red}'` and
	// `PROMPT='A${C}B%F{blue}C '` under `setopt prompt_subst`, both colors
	// arrive — so there the expansion runs first and the table reads its
	// result. The vendor manual says the same in as many words: zsh's
	// prompt string "is first subjected to parameter expansion".
	//
	// It is the difference between a prompt theme working and not. A theme
	// builds its whole prompt out of parameters whose values are `%F{…}`
	// and `%K{…}`; with the table run first there is nothing in the string
	// for it to find, and the escapes arrive afterwards as text nobody will
	// read again — which is exactly what this shell drew.
	ExpandBeforeEscapes bool

	// FailedExpansionKeepsWhatItDrew hands back the text in front of the
	// **first** substitution when the expansion pass gives up partway,
	// instead of the text as it stood when the pass began.
	//
	// The other half of the boundary in promptabandon.go: that one says the
	// script carries on, and this one says what the rendering is worth. Both
	// halves are measured, on the same value in two shells, and they
	// disagree. With `s='PRE-$((nofunc()))-POST'` and the function not
	// registered, zsh's `${(%%)s}` draws `PRE-` — and draws it for
	// `PRE-${V}-$((nofunc()))-POST` too, so the `${V}` that succeeded is
	// **not** kept and the rule is about the first substitution rather than
	// about the failing one. bash's `${v@P}` on the same value draws
	// `PRE-$((nofunc()))-POST`, the text unchanged, with the substitutions
	// simply not performed.
	//
	// The escape pass is not affected either way: measured with `%B` and
	// `\e[1m` in front of the failure, both shells draw the escape and then
	// stop, so what a given-up pass hands back is read by the table exactly
	// as a finished one would be.
	//
	// It is here rather than on [Semantics] because it is a question only a
	// prompt asks: nothing outside a prompt rendering has a *pass* to give
	// up, and a dialect with no prompt language never reads it.
	FailedExpansionKeepsWhatItDrew bool

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

	// Visual is which of those sequences change the terminal's visual state,
	// and how. See [PromptVisual]: it is what lets a bold-off write the
	// color back that clearing the bold took with it.
	//
	// Consulted for Sequences codes only. A Colors code always sets its own
	// layer and never restores, which is measured — `%F{red}%Ba` restores
	// and `%Ba%F{blue}` does not — so there is nothing for a dialect to say
	// about one.
	//
	// A dialect that leaves this empty has no visual state kept for it and
	// its sequences are written as they stand, which is every dialect but
	// one: bash and ksh93 spell their colors as raw escape bytes a person
	// wrote, so there is no code for the shell to know the meaning of.
	Visual map[rune]PromptVisual

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

	// Formats is the codes that take a `strftime` format in braces, in place
	// of the fixed shape their Codes entry draws. zsh spells one:
	// `%D{%H:%M}` is the clock through that format where a bare `%D` is the
	// numeric date.
	//
	// A set rather than a field per code, because the argument is read the
	// same way a color's is and handed to the same resolver — see
	// [Runner.Strftime], which is the formatter `printf '%(fmt)T'` already
	// writes through, so a shell whose embedder pins the clock pins these
	// too.
	//
	// Read after Colors, and only for a code already in Codes: the braces
	// change what the code draws rather than making the code exist. A code
	// in here whose braces are absent draws what its Codes entry says, which
	// is measured — `%D` alone is the plain date.
	Formats map[rune]bool

	// Conditional and ConditionalEnd are the characters that open and close
	// a `%(x.true.false)` — the one code that decides rather than draws, and
	// so the one that is not a row of Codes. zsh spells them `(` and `)`;
	// a dialect that names one names both, and a dialect that names neither
	// has no such construct and never asks Conditions anything.
	//
	// The pair is here rather than assumed from the opening character
	// because the closing one is read in three places — it ends the false
	// arm, it ends a nested construct being skipped, and written after the
	// escape it draws itself — and deriving it would be a rule about
	// parentheses in a table that is otherwise entirely the dialect's.
	Conditional, ConditionalEnd rune

	// Conditions is what each test letter asks. See [PromptCondition] and
	// interp/promptconditional.go, which is the whole of the construct.
	//
	// The table is paired with the refusal the way Codes is: a letter *in*
	// here that this reader cannot answer is refused by name, and a letter
	// not in here is one the dialect's prompt language does not have —
	// measured, zsh draws nothing at all for `%(a.T.F)` and swallows both
	// arms, at status 0 and with nothing on standard error.
	Conditions map[rune]PromptCondition

	// Octal says three octal digits after the escape are the byte they name,
	// which is how bash spells a character it has no letter for.
	//
	// Exactly three, measured: `\007` drew the bell and `\101` drew `A`, while
	// `\0`, `\1`, `\10`, `\00` and `\8` were all left as they were written.
	// A value above 255 is taken low byte first — `\400` drew a NUL.
	Octal bool

	// NumericArgument says a run of digits between the escape and the code is
	// an argument to that code rather than a code of its own.
	//
	// One dialect spells its prompt fields with `%` and takes a count in front
	// of the ones that have components to count; the other spells them `\w`
	// and has no such shape, so this is asked rather than assumed. Without it
	// the walker reads the first digit as the code, finds no field for `2`,
	// and refuses `%2~` by name.
	//
	// The digits are handed to the resolver as the argument, unbraced, and a
	// code that also takes a `{…}` group takes whichever of the two was
	// written — the group where there is one. So the two share one parameter
	// rather than widening the resolver, and the third result of
	// promptArgument is what says which spelling arrived.
	//
	// This used to say that nothing takes both, on the grounds that the codes
	// with a group are the colors and the clock and neither counts anything.
	// The colors do: `%2F` is the color `%F{2}` paints and `%30F` is
	// `%F{30}`, measured, and the count reaching them is #2087. The clock is
	// still the case the sentence described — `%D` counts nothing — but one
	// example is not the rule it was written as.
	NumericArgument bool

	// TrailingEscapeIsDropped says an escape character with nothing after it
	// is removed rather than drawn.
	//
	// Measured on both of one shell's readers and they agree, which is what
	// makes this a row of the table rather than a difference between them:
	// zsh drew `PS1='x%'` as `x` through a pty and `print -P 'x%'` as `x`
	// too, and `${(%)v}` on `%` alone is empty. bash and ksh93 draw the
	// character itself, which is the zero value.
	//
	// It is worth a row because writing the character through is what an
	// implementation that only looks at complete pairs does, and it is
	// silently one character wrong on every prompt that ends in the escape.
	TrailingEscapeIsDropped bool

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

	// NoLoginName is what FieldUser draws when the system has no name for the
	// uid this process runs as — a container started `--user 99999`, and any
	// other uid with no password-database entry.
	//
	// A dialect's own word and not an axis, because the two shells that have
	// the escape simply say different things. Measured 2026-09-12 at uid 99999
	// with no `/etc/passwd` entry, through `PS4` in the bash images and
	// `${(%%)…}` in the zsh one: bash 5.3.15, bash 3.2.57 and bash under an
	// `argv[0]` of `sh` all draw the words `I have no name!`, and zsh 5.9
	// draws nothing at all. So zsh's answer is the zero value here and is
	// asserted in dialect/zsh beside the measurement rather than left to look
	// like a field nobody filled in.
	//
	// This is **not** the same as a Runner nobody told who it is running as.
	// That one is still refused by name — see FieldUser — and the two used to
	// be one empty string, which is what #1451 was: a uid with no entry drew
	// nothing in both dialects, and an empty prompt component announces
	// nothing.
	NoLoginName string

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
	//
	// A dialect that names them also *assigns* them, into PS1 and PS2, in an
	// interactive shell — see [PromptStyle.DefaultsFollowTheStartupFiles] for
	// when. That is what makes `[ -z "$PS1" ] && return` at the top of a
	// person's run-commands file mean what they wrote it to mean: measured
	// through a pty with no rc at all, five of the six panel columns have
	// PS1 set before the file runs and the sixth has it by the time a prompt
	// is drawn, and none of them leaves it empty. A shell that draws a
	// prompt from a value nobody can read has the shape right and the
	// parameter wrong, which is invisible to every probe that watches the
	// screen.
	Default, DefaultContinued string

	// DefaultsFollowTheStartupFiles says this dialect assigns Default and
	// DefaultContinued *after* its startup files rather than before them.
	//
	// The value is the same either way and only the moment differs, which is
	// why it is a row of this table rather than an axis of Semantics: there
	// is no disagreement about whether there is a default.
	//
	// Measured through a pty, `$ENV` pointing at a file that prints
	// `${PS1+set}`, with PS1 unset in the parent. bash 5.3.15, bash 3.2.57,
	// bash under argv[0] `sh`, dash and zsh 5.9.2 all have PS1 in hand while
	// the file runs — `\s-\v\$ `, `$ ` and `%m%# ` respectively. ksh93
	// alone has it *unset* there and reads `$ ` by the time a prompt is
	// drawn, while its PS2 and PS4 are already set. So the one column that
	// waits waits only for PS1, and an rc guarding on PS1 in that shell sees
	// nothing to guard on.
	DefaultsFollowTheStartupFiles bool

	// AssignsWithNobodyToPrompt says this dialect puts PS1 and PS2 in a
	// *non*-interactive shell too, and DefaultWithNobodyToPrompt and
	// DefaultContinuedWithNobodyToPrompt are what it puts there. The two
	// values are read only when the flag is on, which is what lets a dialect
	// assign the empty string — a set-but-empty PS1 and an unset one are
	// different answers, and the guard at the top of a person's rc file is
	// written to tell them apart.
	//
	// The panel gives three answers on `-c` and on a script file alike, with
	// nothing inherited:
	//
	//	bash 5.3.15, bash 3.2.57, bash as `sh`, ksh93   PS1 unset
	//	dash                                            PS1 `$ `, PS2 `> `
	//	zsh 5.9.2                                       PS1 and PS2 set, empty
	//
	// The first row is why this is a flag rather than the default. `[ -z
	// "$PS1" ] && return` is how a real `~/.bashrc` detects that there is
	// nobody to prompt; a shell that assigned a prompt to every script would
	// have the value right and would stop the guard ever firing, which is the
	// same bug as an empty PS1 seen from the other side.
	AssignsWithNobodyToPrompt bool

	// The values for that case. See AssignsWithNobodyToPrompt.
	DefaultWithNobodyToPrompt, DefaultContinuedWithNobodyToPrompt string
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
	// FieldCwd is the working directory with the home directory written `~`
	// and FieldCwdFull is the path untouched. Both count: the argument keeps
	// that many components from the right, or — written with a minus — that
	// many from the left.
	FieldCwd
	FieldCwdFull
	// FieldCwdBase is the last component of the abbreviated path and
	// FieldCwdBaseFull the last component of the untouched one. Neither
	// counts: this is bash's `\W`, which has no numeric argument at all.
	//
	// The two differ only at the home directory itself, and there they differ
	// every time a prompt is drawn there: measured in it, bash's `\W` draws
	// `~` where the directory's own name is what the unabbreviated reading
	// gives. Two readings of one path disagreeing is what says this is two
	// fields.
	FieldCwdBase
	FieldCwdBaseFull
	// FieldCwdCounted and FieldCwdCountedFull are FieldCwd and FieldCwdFull
	// with the count defaulting to **one** instead of to the whole path:
	// zsh's `%c`/`%.` and `%C`.
	//
	// A separate pair rather than a flag on the first two, and separate from
	// FieldCwdBase as well, because all three answers are different and two
	// shells hold them at once. Measured 2026-09-12 in `/tmp`: bash's `\W`
	// draws `tmp`, and zsh's `%c` and `%C` both draw `/tmp` — because a
	// single leading component keeps the `/` in front of it, which is the
	// same rule `%1d` follows and which taking the *basename* cannot express.
	// One directory down all three agree, which is why sharing the field
	// looked right for as long as it did (#1699).
	//
	// `%Nc` for N of 1 or more is exactly `%N~`, and `%0c` is `%1c`; the only
	// thing separating `%c` from `%~` is what an absent count means.
	FieldCwdCounted
	FieldCwdCountedFull
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
	// FieldSourceFile is the file being read: the sourced file, the script,
	// or — under `-c`, where there is no file — what the shell calls itself.
	// FieldUnitName is the *name* of the function, sourced file or script
	// being read, where FieldSourceFile stays a function's defining file.
	//
	// The two a script reaches for, and the reason this table needed them:
	// they were the whole of the four escapes this package carried before
	// there was one table, and a merge that dropped them would have turned
	// `${(%):-%x}` — the wild idiom for a file's own path — from an answer
	// into a silently dropped code.
	FieldSourceFile
	FieldUnitName
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
	// FieldCountedColumn draws nothing and occupies a column. zsh spells it
	// `%G`, and it is what a prompt uses to tell the shell that bytes the
	// markers above have hidden do reach the screen after all — measured,
	// `%G` alone leaves the text empty and the line one column along, and
	// `%{a%Gb%}`, where the markers say nothing is drawn, is one column
	// rather than none.
	//
	// A field rather than a Sequences entry because a sequence is bytes the
	// terminal reads and never a column, which is exactly the distinction
	// this code exists to cross.
	FieldCountedColumn
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

// PromptAttribute is one setting of the terminal's visual state, for the
// walker to keep while it draws.
//
// It has to keep one, because on this terminal there is no way to turn a
// single attribute off. Measured against zsh 5.9.2 under
// `TERM=xterm-256color`, `%b` writes `\e[0m` — select-graphic-rendition
// nought, which clears *everything*, colors included — and then writes the
// color that was in effect again, so the text after it is still red:
//
//	${(%%)'%F{red}%Bbold%b still'}
//	\e[31m \e[1m\e[31m bold \e[0m\e[31m  still
//
// Without the second half a prompt loses its color at the first bold-off,
// which a theme that writes `%b%k` between segments does on every segment.
//
// The names are the terminal's rather than a shell's, in the way
// [PromptColor] and the numbers in promptcolor.go already are: bold,
// standout and underline are terminfo's `bold`, `smso` and `smul`, and a
// second dialect with a visual language gets the machinery without saying
// any of it again.
//
// **The constants are in the order a restore writes them**, which is
// measured rather than chosen: `%U%S%F{red}%K{blue}a%b` restored
// `\e[7m\e[4m\e[31m\e[44m` — standout before underline whichever order they
// were turned on in — and `%B%S...%u` restored `\e[1m\e[7m`.
type PromptAttribute int

const (
	// AttributeNone is the zero value, so a [PromptVisual] that names no
	// attribute changes none. A code can still restore without owning a
	// setting of its own; nothing in the panel does, and the zero value is
	// what says so.
	AttributeNone PromptAttribute = iota
	AttributeBold
	AttributeStandout
	AttributeUnderline
	// AttributeForeground and AttributeBackground are the two [PromptColor]
	// layers, as settings to be restored. A Colors code sets its own layer
	// without being named here; these are for the codes that *clear* one,
	// which a dialect spells as a fixed sequence — zsh's `%f` is `\e[39m` —
	// and which the walker would otherwise have no way to know had cleared
	// anything.
	AttributeForeground
	AttributeBackground
)

// attributeOf is the setting a color layer occupies. The two enumerations
// are parallel by construction rather than by upkeep, which this is the
// only place that needs.
func attributeOf(layer PromptColor) PromptAttribute {
	if layer == Background {
		return AttributeBackground
	}
	return AttributeForeground
}

// PromptVisual is what one [PromptStyle.Sequences] code does to the terminal's
// visual state, for the dialects whose visual language has codes that disturb
// more than they name.
//
// A code not in [PromptStyle.Visual] is bytes and nothing more, which is what
// every code was before this and what most still are.
type PromptVisual struct {
	// Attribute is the setting this code owns. Setting it records the
	// code's own sequence as what is in effect; see Off for the other half.
	Attribute PromptAttribute

	// Off says the code clears its attribute rather than setting it.
	Off bool

	// Restores says the sequence disturbs settings other than its own, so
	// everything still in effect is written again after it — in the order
	// the [PromptAttribute] constants are in, and skipping this code's own
	// attribute, which the code has just written for itself.
	//
	// Which codes need it is measured and is not the tidy answer. Against
	// zsh 5.9.2 the four that restore are `%B`, `%b`, `%u` and `%s` —
	// bold-on and the three attribute-offs — while `%U`, `%S`, `%f`, `%k`,
	// `%F{…}` and `%K{…}` write their sequence and nothing else:
	//
	//	%F{red}%Ba   \e[31m \e[1m\e[31m a      bold-on restores
	//	%F{red}%Ua   \e[31m \e[4m a           underline-on does not
	//	%B%F{red}a%f \e[1m\e[31m a \e[39m     foreground-off does not
	//	%B%Sa%u      \e[1m\e[7m a \e[24m\e[1m\e[7m
	//
	// So it is a row of the table rather than a rule over it: a reader that
	// derived it from "this sequence turns something off" would write the
	// restore after `%f` too, and one that derived it from "this sequence
	// resets everything" would leave it off `%u`, whose `\e[24m` resets
	// nothing but the underline.
	Restores bool
}

// PromptResolver is what one code draws, for the reader that holds the facts.
//
// arg is what stood in braces after the code — a color, or a `strftime`
// format — and braced says whether there were braces at all. The two are a
// measured difference and not belt and braces: zsh draws `%D` as the plain
// date and `%D{}` as nothing, so an empty format is not the absence of one.
// A color code reads them the same way round — `%F` and `%F{}` both draw
// `\e[30m` — which is why the distinction is the resolver's to make.
//
// The second result says whether this reader has an answer at all. False is
// not an error: it is the honest half of the split this file opens with, and
// the caller decides what to do with it. A script's expansion refuses the
// escape by name; a prompt drawer falls through to the table's Unknown.
type PromptResolver func(f PromptField, arg string, braced bool) (string, bool)

// ExpandPromptStyle walks a prompt's text and draws each code in it.
//
// The one walker over the one table, called by the prompt drawer and by the
// `%` expansion flag alike — so `print -P '%F{196}red%f'` and a drawn prompt
// agree by construction rather than by two implementations staying in step.
//
// The string result is the escape the resolver had no answer for, spelled the
// way a script wrote it — `e` for a plain code and `(e` for a conditional's
// test letter — and is meaningful only when ok is false. The text returned
// with it is what had been drawn up to that point, which no caller uses and
// which is returned rather than dropped so that a caller wanting to report
// *where* in the prompt it stopped can.
//
// quantity answers the conditional's tests and may be nil for a reader with
// no answers to them; a style with no [PromptStyle.Conditional] never asks it.
func ExpandPromptStyle(st PromptStyle, text string, field PromptResolver, quantity PromptQuantityResolver) (string, string, bool) {
	var visual promptVisualState
	return expandPromptStyle(st, text, field, quantity, &visual)
}

// expandPromptStyle is [ExpandPromptStyle] over a visual state the caller
// keeps, which is every caller inside this package: the state is the shell's
// and outlives one rendering. See [promptVisualState].
func expandPromptStyle(st PromptStyle, text string, field PromptResolver, quantity PromptQuantityResolver, visual *promptVisualState) (string, string, bool) {
	if st.Escape == 0 || text == "" {
		return text, "", true
	}
	w := promptWalk{st: st, field: field, quantity: quantity, width: unaskedWidth, visual: *visual}
	w.walk([]rune(text))
	// Written back whether or not the walk was refused: what it drew before
	// the refusal has reached the terminal, so the sequences it wrote are in
	// effect either way.
	*visual = w.visual
	return w.b.String(), w.refused, w.refused == ""
}

// RenderPromptValue turns a prompt parameter's value into the text it stands
// for: the escape table over it, and parameter and command expansion over it,
// in the order this style draws them.
//
// One place rather than three, which is the point. There are three readers of
// the same value — the prompt drawer, `PS4` in front of a trace line, and
// bash's `${v@P}` — and each of them has to run *both* passes in
// [PromptStyle.ExpandBeforeEscapes]'s order. Written out per reader, a reader
// added later gets one pass, or gets both in the order its author assumed;
// this repository's recurring failure is a second helper that omits half of a
// rule the first one carries, and an ordering measured in both directions is
// exactly the kind of rule that goes missing.
//
// The style is a parameter rather than read from the runner because the
// drawer holds its own copy — a front end may draw with a table the
// interpreter was never given — and so are the resolvers, which is what the
// three readers really differ by: the drawer answers every code, a trace
// falls through to the dialect's policy for an unknown one, and a script's
// expansion refuses by name what the runner has no answer for.
//
// A refusal stops both passes. The second result is the escape that was
// refused, spelled as [ExpandPromptStyle] spells it, and the text comes back
// as it was written: there is no drawing on past an escape whose value is
// unknown, and no running a command substitution for a value that is going to
// be refused either. A reader with a resolver that answers everything never
// sees it.
//
// r may be nil, which is a value with no shell behind it: the escapes are
// drawn and nothing is expanded.
func RenderPromptValue(st PromptStyle, r *Runner, text string, field PromptResolver, quantity PromptQuantityResolver) (string, string, bool) {
	expand := func(v string) string {
		if r == nil || st.Expand == nil || !st.Expand(r) {
			return v
		}
		// Not Runner.Expand: this pass is a boundary, and a failure inside
		// it costs the rendering rather than the script. See
		// interp/promptabandon.go, which is where the one catch lives so
		// that every reader of this helper gets it.
		return r.expandPromptText(v)
	}
	escapes := func(v string) (string, string, bool) {
		visual := &promptVisualState{}
		if r != nil {
			visual = &r.promptVisual
		}
		return expandPromptStyle(st, v, field, quantity, visual)
	}
	if st.ExpandBeforeEscapes {
		return escapes(expand(text))
	}
	out, code, ok := escapes(text)
	if !ok {
		return text, code, false
	}
	return expand(out), "", true
}

// promptWalk is the walker's state: what has been drawn, and where on the
// line it has reached.
//
// The column is here rather than counted afterwards because a prompt can ask
// about it *while* it is being drawn — `%(l.…)` is "at least n columns have
// been printed on this line", and powerlevel10k's own prompt-length routine
// binary-searches on it — so the count has to be the one this walk has
// produced so far and not one taken from the finished text. Measured, and it
// is the whole reason this is a struct: `ab%2(l.T.F)cd%4(l.T.F)` draws `abTcdT`,
// so the first conditional's own output is part of what the second one counts.
type promptWalk struct {
	st       PromptStyle
	field    PromptResolver
	quantity PromptQuantityResolver
	b        strings.Builder

	// col is how many columns have been drawn on the current line, and width
	// is what the line wraps at — asked of the reader once, and lazily, so a
	// prompt with no conditional in it never asks at all.
	col   int
	width int

	// hidden is the depth of `%{ … %}`, where what is drawn occupies no
	// column: measured, `%{XY%}ab` has drawn two columns and not four.
	hidden int

	// visual is the terminal's visual state as the shell has set it — see
	// [promptVisualState], which this walk is handed and hands back.
	visual promptVisualState

	// refused is the escape this reader had no answer for, empty until one is
	// met. It stops the walk: there is no drawing on past an escape whose
	// value is unknown, because whatever follows would be in the wrong place.
	refused string
}

// promptVisualState is the sequence currently in effect for each
// [PromptAttribute], empty where nothing is. It is what a restoring code
// writes back — see [PromptVisual.Restores].
//
// **It belongs to the shell and not to the walk.** A rendering leaves the
// terminal however its last code left it, and the next rendering restores
// from there, which is measured on zsh 5.9.2 under `TERM=xterm-256color`:
//
//	v=%F{070}; w=%b
//	print -rn -- "${(%%)v}"; print -rn -- "${(%%)w}"
//	\e[38;5;70m  \e[0m\e[38;5;70m
//
// The `%b` is alone in its own rendering and still writes the color back,
// because the `%F{070}` of the rendering before it is still in effect. Two
// separate walks each starting empty answer `\e[0m` and lose the color — and
// that is not an edge: powerlevel10k measures its own width through
// `${(%%)…}` many times per prompt, so by the time the prompt itself is drawn
// the state is never empty, and every `%b%k%F{…}` between its segments came
// out one escape short (#2113).
//
// It accumulates rather than being replaced: `%U` in one rendering and
// `%K{021}` in another are both written back by a `%b` in a third. What
// clears an entry is a code that turns that attribute off — zsh's `%f` is
// `\e[39m` and leaves the foreground with nothing to restore.
//
// A subshell's renderings do not reach the parent, which is [Runner.clone]'s
// answer already: this is a value in the struct the clone copies, so
// `(print -rn -- "${(%%)v}")` leaves the parent's state alone, as measured.
type promptVisualState [AttributeBackground + 1]string

// unaskedWidth is a width no terminal has, so that nought — which is a width
// a reader really does report, and which zsh treats as its own case — is not
// mistaken for "not asked yet".
const unaskedWidth = -1 << 30

// walk draws one run of prompt text, which is the whole of it or one arm of a
// conditional. Recursive for the arm, so that the column an arm draws is the
// same column the next conditional counts.
func (w *promptWalk) walk(runes []rune) {
	for i := 0; i < len(runes); i++ {
		if w.refused != "" {
			return
		}
		if runes[i] != w.st.Escape {
			w.draw(string(runes[i]))
			continue
		}
		if i+1 >= len(runes) {
			// An escape with nothing after it. There is no code to look up,
			// so none of the answers below applies — and the two readers
			// measured *differ* here, which is why the table decides: a
			// prompt draws the character itself and the expansion flag drops
			// it. See PromptStyle.TrailingEscapeIsDropped.
			if !w.st.TrailingEscapeIsDropped {
				w.draw(string(runes[i]))
			}
			return
		}
		// The count in front of the code, where the dialect takes it. A run
		// that reaches the end of the text has no code to be an argument to,
		// and draws nothing: measured, a bare `%2` is the empty string in zsh
		// 5.9.2, which is neither the digits nor a refusal (#1592).
		num, j := w.countAt(runes, i+1)
		if j >= len(runes) {
			return
		}
		i = j
		code := runes[i]
		if w.st.Conditional != 0 && code == w.st.Conditional {
			i = w.conditional(runes, i, num)
			continue
		}
		if w.st.Conditional != 0 && code == w.st.ConditionalEnd {
			// The character that closes a conditional, written after an
			// escape, is itself — which is how an arm holds one at all.
			// Measured, and it is not only an arm's rule: `a%)b` draws `a)b`
			// wherever it stands, and the column counts it.
			w.draw(string(code))
			continue
		}
		if fld, ok := w.st.Codes[code]; ok {
			arg, braced := num, false
			if w.st.Formats[code] {
				var next int
				group, next, hasGroup := promptArgument(runes, i+1)
				i = next
				if hasGroup {
					arg, braced = group, true
				}
			}
			v, answered := w.field(fld, arg, braced)
			if !answered {
				w.refused = string(code)
				return
			}
			switch fld {
			case FieldNonPrintingStart:
				w.hidden++
			case FieldNonPrintingEnd:
				if w.hidden > 0 {
					w.hidden--
				}
			case FieldCountedColumn:
				// Draws nothing and occupies a column, which is what it is
				// for: measured, `%G` alone leaves the text empty and puts
				// the line one column along, and `%{a%Gb%}` — where the
				// markers say nothing is drawn — is one column and not none.
				w.b.WriteString(v)
				w.cell(' ')
				continue
			}
			w.draw(v)
			continue
		}
		if seq, ok := w.st.Sequences[code]; ok {
			// Bytes the terminal reads rather than draws, so they are written
			// and not counted: measured, `%F{red}abc` has drawn three
			// columns.
			w.b.WriteString(seq)
			w.visualWritten(code, seq)
			continue
		}
		if layer, ok := w.st.Colors[code]; ok {
			// A count in front of a color code is the index it paints, and
			// braces after it win over the count: measured, `%2F` is
			// `\e[32m`, `%30F` is `\e[38;5;30m` and `%2F{red}` is `\e[31m`.
			// The two are read here in the order the Codes branch above reads
			// them, and for the same reason — a code takes one argument, and
			// which spelling it arrived in is what the third result says.
			//
			// This branch dropped the count until #2087, so every unbraced
			// numeric color drew the *empty* argument, which colorIndex reads
			// as nought and paints black. A wrong color at status 0 with
			// nothing said.
			arg, next, braced := promptArgument(runes, i+1)
			i = next
			if !braced {
				if promptCount(num) < 0 {
					// A *negative* index is not a color out of range, it is
					// no color at all: measured, `%-2F` and `%-1F` write
					// nothing whatever — where `%F{-1}` writes the terminal's
					// default. So the two readings of the argument part
					// company here and only here, and a bare `%-F` is minus
					// one and writes nothing too.
					//
					// It still **clears the layer**, which is the half that
					// cannot be inferred from "it writes nothing" and is
					// measured on its own: `%F{red}a%-2Fb%b` restores nothing
					// after the reset, where `%F{red}ab%b` restores the red —
					// and `%K{blue}%F{red}%-2Ka%b` restores the foreground
					// alone, so it is that layer and not both. #1699.
					w.visual[attributeOf(layer)] = ""
					continue
				}
				arg = num
			}
			seq := colorSequence(layer, arg)
			w.b.WriteString(seq)
			// A color code is the one that is always a setting: it names the
			// layer it paints, so the walker knows what is in effect without
			// the dialect saying anything. It never restores; see
			// PromptStyle.Visual.
			w.visual[attributeOf(layer)] = seq
			continue
		}
		if v, ok := promptOctalByte(w.st.Octal, runes, i); ok {
			// The byte itself and not the rune it names: `\\377` is one byte
			// on the wire, where writing it as a rune would be the two bytes
			// its UTF-8 spelling takes.
			w.b.WriteByte(v)
			i += 2
			continue
		}
		// In no part of the table. Handed to the resolver as FieldNone —
		// "this table has no field for it" — with the code as the argument,
		// because the two readers want different things from it and both are
		// right.
		//
		// A **prompt** has to draw something, and what it draws is the
		// dialect's Unknown: bash writes both characters, ksh93 drops the
		// escape and zsh drops the pair. A **script's expansion** has no such
		// obligation and refuses the escape **by name**, which is the
		// convention that let this shell's prompt-escape surface be
		// enumerated exactly rather than guessed at — `${(%):-%q}` says
		// which escape it could not answer, where the shell it copies drops
		// it silently at status 0. A prompt quietly short of a field is the
		// kind of wrong answer nobody reports, and dropping a code because
		// the *drawer's* policy says to would have made every unbuilt escape
		// silent the moment there was one table.
		//
		// So Unknown is read by the resolver that has to obey it rather than
		// by this walker, and the refusal is read by the one that can afford
		// it.
		v, answered := w.field(FieldNone, string(code), false)
		if !answered {
			w.refused = string(code)
			return
		}
		w.draw(v)
	}
}

// visualWritten records what a sequence did to the terminal's visual state,
// and writes back whatever it disturbed on the way.
//
// This is the whole of the restore, and it is here — in the walker both
// readers share — rather than in a second pass over the finished text,
// because the sequence to write back is the one that was in effect *at that
// point* in the walk. A pass over the result would have to parse the escapes
// back out to know, which is reading our own output to learn what we meant.
func (w *promptWalk) visualWritten(code rune, seq string) {
	v, ok := w.st.Visual[code]
	if !ok {
		return
	}
	if v.Attribute != AttributeNone {
		if v.Off {
			w.visual[v.Attribute] = ""
		} else {
			w.visual[v.Attribute] = seq
		}
	}
	if !v.Restores {
		return
	}
	for a := AttributeBold; a <= AttributeBackground; a++ {
		// Its own attribute is skipped rather than written twice: this code
		// has just written it, and measured, `%F{red}%B%Ba` is
		// `\e[31m \e[1m\e[31m \e[1m\e[31m` — one bold per `%B` and the color
		// after each, never `\e[1m\e[1m`.
		if a == v.Attribute {
			continue
		}
		w.b.WriteString(w.visual[a])
	}
}

// draw writes text the terminal shows, and counts what it costs.
func (w *promptWalk) draw(v string) {
	w.b.WriteString(v)
	if w.st.Conditional == 0 || w.hidden > 0 {
		// Nothing can ask about the column, or nothing drawn here reaches
		// one. Either way the arithmetic below is work with no reader.
		return
	}
	for _, r := range v {
		w.cell(r)
	}
}

// promptArgument reads the braces after a code, and says where the code
// ended.
//
// No braces is the empty argument and the code ends where it was, which is
// measured: zsh drew `%Fred` as the color for an empty argument and then the
// three letters, so the letters after an unbraced code are text.
//
// An opening brace with no closing one is the rest of the prompt, for the
// reason an unterminated anything is: there is no later text for it to be
// text of.
func promptArgument(runes []rune, i int) (string, int, bool) {
	if i >= len(runes) || runes[i] != '{' {
		return "", i - 1, false
	}
	for j := i + 1; j < len(runes); j++ {
		if runes[j] == '}' {
			return string(runes[i+1 : j]), j, true
		}
	}
	return string(runes[i+1:]), len(runes) - 1, true
}

// promptOctalByte reads three octal digits as the byte they name.
//
// Named apart from transform.go's octalByte, which goes the other way: that
// one writes a byte as `\NNN` for a quoted listing, and this one reads three
// digits a prompt was written with.
//
// Three exactly. Measured against bash: `\007` drew the bell and `\101` drew
// `A`, while `\0`, `\1`, `\10`, `\00` and `\8` were each drawn as the two
// characters written — so a shorter run is not a shorter number, it is not a
// number at all and falls through to whatever the dialect does with a code it
// does not know.
//
// The low byte of the value, so `\400` is a NUL, which is what bash drew.
func promptOctalByte(enabled bool, runes []rune, i int) (byte, bool) {
	if !enabled || i+2 >= len(runes) {
		return 0, false
	}
	v := 0
	for _, r := range runes[i : i+3] {
		if r < '0' || r > '7' {
			return 0, false
		}
		v = v*8 + int(r-'0')
	}
	return byte(v), true
}

// What a prompt's color code writes to the terminal.
//
// The dialect says which half of the screen its code paints and what was
// written in the braces; everything below is the terminal's own arithmetic —
// the select-graphic-rendition parameters, where 30 to 37 are the eight
// foreground colors, 90 to 97 their bright halves, 39 the default, `38;5;n`
// the 256-color extension and `38;2;r;g;b` the direct-color one. Those
// numbers belong to the terminal in the same way the cell widths in
// repl/cellwidth.go do, so a second dialect with a color code gets them
// without saying them again.
//
// Measured against zsh 5.9.2, `TERM=xterm-256color`, 93 arguments in both
// layers — the whole grid is in interp/promptcolor_test.go and the rule it
// yields is in colorIndex below.
//
// **One thing here is deliberately not the shell's answer, and it is
// recorded rather than matched: the count of colors the *terminal* claims.**
// Measured, `%F{9}` is `\e[91m` under `TERM=xterm-256color` and `\e[39m` —
// the default — under `TERM=xterm`, which reports eight; `%F{200}` is
// `\e[38;5;200m` and `\e[39m` the same way round. So an index above seven is
// answered by asking terminfo how many colors there are, and below eight it
// is not. This models the 256-color terminal unconditionally, which is what
// every terminal a person runs this in reports, and it is the wrong answer
// on a genuinely eight-color one. Matching it means reading terminfo, which
// is a capability database rather than a shell behavior and is a seam this
// substrate has not got — the `#rrggbb` form below is the evidence that the
// two questions are separate, since it draws the same direct-color sequence
// under *every* TERM including `dumb`.
func colorSequence(layer PromptColor, arg string) string {
	if len(arg) > 0 && arg[0] == '#' {
		if rgb, ok := directColor(arg); ok {
			// The direct-color form, which is its own answer rather than an
			// index into anything: `%F{#ff8800}` drew `\e[38;2;255;136;0m`
			// and `%K{#ff8800}` drew `\e[48;2;…`, and both did so under
			// every TERM measured.
			return "\x1b[" + strconv.Itoa(colorExtended(layer)) + ";2;" +
				strconv.Itoa(rgb[0]) + ";" + strconv.Itoa(rgb[1]) + ";" +
				strconv.Itoa(rgb[2]) + "m"
		}
		// A `#` that is not that form splits two ways, and the split is
		// measured rather than tidy: a run of hex digits that *starts* badly
		// is no number at all and comes to nought, which is the first color
		// — `%F{#}`, `%F{#g}` and `%F{#ggg}` all drew `\e[30m`. A run that
		// starts well and is the wrong length or ends badly is a malformed
		// number, and that is the default — `%F{#0}`, `%F{#0000}` and
		// `%F{#00g}` drew `\e[39m`.
		if len(arg) > 1 && isHexDigit(arg[1]) {
			return sgr(defaultColor(layer))
		}
		return sgr(colorBase(layer))
	}
	n, ok := colorIndex(arg)
	if !ok {
		// Anything the names and the number range both refuse is the
		// terminal's default. Measured: `%F{bogus}`, `%F{Red}` — the names are
		// lower case and only lower case — `%F{256}` and `%F{-1}` all drew
		// `\e[39m`, which is the same answer `%f` gives.
		return sgr(defaultColor(layer))
	}
	switch {
	case n < 8:
		return sgr(colorBase(layer) + n)
	case n < 16:
		// The bright half, which is its own run of parameters rather than a
		// modifier on the first eight.
		return sgr(colorBright(layer) + n - 8)
	default:
		return "\x1b[" + strconv.Itoa(colorExtended(layer)) + ";5;" + strconv.Itoa(n) + "m"
	}
}

// directColor reads the `#rrggbb` and `#rgb` forms, and reports whether the
// argument was one at all.
//
// Three hex digits or six, and each of the three doubled in the short form:
// `%F{#fff}` drew `38;2;255;255;255` and `%F{#abc}` drew `38;2;170;187;204`.
// No other length is one — `#0`, `#00`, `#0000`, `#f`, `#ff`, `#ffff` and
// `#fffff` all drew the plain default — and the case of the letters does not
// matter, since `#FF8800` and `#ff8800` drew the same bytes.
//
// The second result is false for an argument that is not this form, which
// includes a `#` whose digits are not hex at all: `%F{#ggg}` and `%F{#}` drew
// `\e[30m`, the *first* color, where `%F{#00g}` drew the default. That split
// is measured and is the reason this counts the leading hex digits rather
// than only checking the length — a run that starts badly is no number, and
// falls to colorIndex's reading of a string with no digits in it, which is
// nought. A run that starts well and ends badly is a malformed number, and
// that is the default.
// isHexDigit and hexValue are builtin.go's, shared rather than written twice:
// `$'\x41'` and `%F{#ff8800}` read the same digits.
func directColor(arg string) ([3]int, bool) {
	if len(arg) == 0 || arg[0] != '#' {
		return [3]int{}, false
	}
	h := arg[1:]
	digits := 0
	for digits < len(h) && isHexDigit(h[digits]) {
		digits++
	}
	if digits != len(h) || (digits != 3 && digits != 6) {
		return [3]int{}, false
	}
	var rgb [3]int
	for i := range rgb {
		if digits == 3 {
			// Each digit doubled, which is the usual reading and is what was
			// measured: `#f` in a channel is 255 and not 15.
			v := hexValue(h[i])
			rgb[i] = v*16 + v
			continue
		}
		rgb[i] = hexValue(h[2*i])*16 + hexValue(h[2*i+1])
	}
	return rgb, true
}

// colorIndex reads an argument that is not the direct-color form as an index
// into the terminal's palette, and reports whether it is one at all.
//
// The rule, derived from 93 measured arguments in both layers rather than
// guessed at, and it is three readings rather than one:
//
//	#…       the direct-color form and its two malformed readings, all three
//	         answered by colorSequence before this is called.
//	a letter a *name*, matched by prefix.
//	anything the digits at the front of it, and nought where there are none.
//
// The middle one is the surprise and it is measured twice over: `%F{re}` drew
// red and `%F{b}` drew *black* rather than being ambiguous with blue, so a
// prefix matches the first of the eight in the order the terminal numbers
// them. The name ends at the first character that is not a letter — `%F{red,}`,
// `%F{red bold}` and `%F{red;bold}` all drew red — and a run of letters that
// is not a prefix of any of them is the default, which is why `%F{bogus}`,
// `%F{grey}` and `%F{x9}` draw no color while `%F{,red}` draws black.
//
// The last reading is C's `strtol` and is measured as such: leading
// whitespace is skipped (`%F{  9}` is 9), a sign is taken (`%F{+9}` is 9),
// trailing junk is ignored (`%F{9x}` is 9 and `%F{1red}` is 1), and a string
// with no digits at the front is nought rather than an error — which is what
// makes `%F{-}`, `%F{,}`, `%F{ }`, `%F{0x9}` and `%F{ red }` all draw the
// first color. Out of the range 0 to 255 is the default: `%F{256}` and
// `%F{-1}` drew `\e[39m`.
func colorIndex(arg string) (int, bool) {
	if len(arg) > 0 && isLetter(arg[0]) {
		return colorNamed(arg)
	}
	n, ok := decimalAtTheFront(arg)
	if !ok || n < 0 || n >= TerminalColors {
		return 0, false
	}
	return n, true
}

// TerminalColors is how many colors this shell's own color codes can name:
// the indices 0 to 255, which colorIndex above accepts and colorSequence
// paints.
//
// Exported because it is the answer to a question a script can ask directly.
// `$terminfo[colors]` and `$termcap[Co]` are that question — a prompt reads
// one of them and picks a palette by it — and the count they report has to be
// the count this shell will actually paint, or a theme takes the 256-color
// branch on a shell whose `%F{200}` draws the default. One constant, read by
// the renderer and by the capability alike, rather than a second `256`
// somewhere that agrees today.
//
// It is deliberately **not** the terminal's own count, and the note above
// colorSequence is where that choice is recorded: measured, real zsh answers
// 8 under `TERM=xterm` and nothing at all under `TERM=dumb`, because it reads
// terminfo. This shell paints the 256-color sequences under every TERM
// including `dumb`, so 256 is what it has to report — the number is a fact
// about this shell, arrived at honestly, rather than a claim about the
// screen.
const TerminalColors = 256

// colorNamed matches a run of letters against the eight names by prefix, the
// first in the terminal's own numbering winning an ambiguous one.
func colorNamed(arg string) (int, bool) {
	end := 0
	for end < len(arg) && isLetter(arg[end]) {
		end++
	}
	name := arg[:end]
	for i, full := range colorNames {
		if strings.HasPrefix(full, name) {
			return i, true
		}
	}
	return 0, false
}

// colorNames is the eight the terminal names, in the order it numbers them.
//
// A slice and not a map, because the order decides an ambiguous prefix:
// `%F{b}` drew black and not blue.
var colorNames = [8]string{
	"black", "red", "green", "yellow",
	"blue", "magenta", "cyan", "white",
}

// decimalAtTheFront reads the number a C `strtol` would: leading whitespace,
// an optional sign, then digits, and whatever follows is ignored.
//
// The second result is false only for an overflow that cannot be a color
// anyway. No digits at all is nought and *true*, which is measured — see
// colorIndex — and is the one place this differs from every other numeric
// reading in this package.
func decimalAtTheFront(arg string) (int, bool) {
	i := 0
	for i < len(arg) && (arg[i] == ' ' || arg[i] == '\t' || arg[i] == '\n') {
		i++
	}
	sign := 1
	if i < len(arg) && (arg[i] == '+' || arg[i] == '-') {
		if arg[i] == '-' {
			sign = -1
		}
		i++
	}
	n := 0
	for i < len(arg) && arg[i] >= '0' && arg[i] <= '9' {
		n = n*10 + int(arg[i]-'0')
		if n > 1<<20 {
			// Far past any color, and stopping here keeps the loop from
			// overflowing on a long run of digits.
			return 0, false
		}
		i++
	}
	return sign * n, true
}

func colorBase(layer PromptColor) int {
	if layer == Background {
		return 40
	}
	return 30
}

func colorBright(layer PromptColor) int {
	if layer == Background {
		return 100
	}
	return 90
}

func colorExtended(layer PromptColor) int {
	if layer == Background {
		return 48
	}
	return 38
}

func defaultColor(layer PromptColor) int {
	if layer == Background {
		return 49
	}
	return 39
}

func sgr(n int) string { return "\x1b[" + strconv.Itoa(n) + "m" }

// promptField is this reader's half of the split: what a Runner can answer
// about a prompt code, and what it refuses by name.
//
// A Runner holds a working directory, a status, a job table, a clock and the
// file it is reading, so it answers those. It does not hold a session: there
// is no history to number, no terminal to name, and nothing open in a script
// that has already parsed. Those are refused, not guessed — a prompt quietly
// short of a field is the kind of wrong answer nobody reports, which is the
// reason the by-name refusal was the whole of this package's answer before
// there was one table.
//
// Three answers here differ from the drawer's on purpose, and each is
// measured against real zsh:
//
//	%_       the open state, which is empty in a script — `print -P '%_'` is
//	         empty, and the drawer names the construct being continued.
//	%{ %}    nothing at all — `print -P '%{X%}'` is `X` — where the drawer
//	         needs its width markers there.
//	newline  `\n`, where a prompt drawn in raw mode needs `\r\n` with it.
//
// askPromptUser is the login name, asked of whoever carried it in, and whether
// anybody carried one in at all.
//
// One accessor rather than a nil check at each use, because a reader that
// forgot the check would panic on every runner nobody told, which is most of
// them.
//
// **The two empties are told apart, and that is the whole of #1451.** They used
// to be one string: a Runner nobody told and a uid the password database has no
// entry for both answered `""`, and the caller refused on the string without
// having to know which it met. They want different answers — the first is a
// gap in how this Runner was set up and is still refused by name, the second is
// a real state of a real machine and is the dialect's own word for it — so the
// second result says which, and only the caller decides what either means.
func (r *Runner) askPromptUser() (string, bool) {
	if r.promptUser == nil {
		return "", false
	}
	return r.promptUser(), true
}

// askPromptHost is the machine's name, asked the same way and for the same
// reason.
func (r *Runner) askPromptHost() string {
	if r.promptHost == nil {
		return ""
	}
	return r.promptHost()
}

// arg is the braces after the code, for the Formats entries.
func (r *Runner) promptField(f PromptField, arg string, braced bool) (string, bool) {
	st := r.promptStyle
	switch f {
	case FieldEscape:
		return string(st.Escape), true
	case FieldUser:
		// Answered only where somebody told this runner who that is
		// (SetPromptUser); a runner nobody told refuses it with the rest
		// rather than expanding to nothing, which would be a wrong answer
		// wearing a success.
		name, told := r.askPromptUser()
		if !told {
			return "", false
		}
		if name == "" {
			// Told, and the system had no answer — a uid with no
			// password-database entry. That is a fact about the machine
			// rather than a gap in this Runner, and what to draw for it is
			// the dialect's: bash says so in words and zsh says nothing.
			// #1451.
			return st.NoLoginName, true
		}
		return name, true
	case FieldHost:
		full := r.askPromptHost()
		host, _, _ := strings.Cut(full, ".")
		return host, full != ""
	case FieldHostFull:
		full := r.askPromptHost()
		return full, full != ""
	case FieldCwd:
		return countedComponents(abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME")), arg, 0), true
	case FieldCwdFull:
		return countedComponents(r.promptVar("PWD"), arg, 0), true
	case FieldCwdBase:
		return lastPathComponent(abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME"))), true
	case FieldCwdBaseFull:
		return lastPathComponent(r.promptVar("PWD")), true
	case FieldCwdCounted:
		return countedComponents(abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME")), arg, 1), true
	case FieldCwdCountedFull:
		return countedComponents(r.promptVar("PWD"), arg, 1), true
	case FieldPrivilege:
		// A read of the process's identity, which is the class .golangci.yml
		// blesses beside `$$` and `$UID`: nothing a script does changes it,
		// and two Runners in one program genuinely share it.
		if os.Geteuid() == 0 {
			return "#", true
		}
		return st.Privilege, true
	case FieldShellName:
		return path.Base(r.name()), true
	case FieldNewline:
		return "\n", true
	case FieldReturn:
		return "\r", true
	case FieldTab:
		return "\t", true
	case FieldSourceFile:
		if fl := r.currentFile(); fl != "" {
			return fl, true
		}
		return r.name(), true
	case FieldUnitName:
		return r.promptUnitName(), true
	case FieldOpenState:
		// Nothing is open: a script that reached an expansion has parsed.
		return "", true
	case FieldVersion:
		return st.Version, st.Version != ""
	case FieldVersionFull:
		return st.VersionFull, st.VersionFull != ""
	case FieldJobCount:
		n := 0
		for _, j := range r.Jobs() {
			if !j.Finished() {
				n++
			}
		}
		return itoa(n), true
	case FieldExitStatus:
		return itoa(r.ExitStatus()), true
	case FieldNonPrintingStart, FieldNonPrintingEnd:
		// Nothing, measured: `print -P '%{X%}'` is `X` and neither marker
		// reaches the output. The drawer puts its width markers here instead.
		return "", true
	case FieldCountedColumn:
		// Nothing either, and both readers agree: what this code is for is
		// the *column* it occupies, which the walker counts rather than the
		// resolver. Measured, `${(%):-a%Gb}` is `ab`.
		return "", true
	}
	if v, ok := r.promptClockField(f, arg, braced); ok {
		return v, true
	}
	// The session's own facts, which a Runner has not got: the history
	// number, how many commands this session has run, and the terminal's
	// name. Refused by name rather than answered with a plausible zero.
	return "", false
}

// promptQuantity is this reader's half of the conditional's split: what a
// Runner can count, and what it refuses by name.
//
// The same line the fields are split along, and drawn from the same state —
// a Runner holds a status, a job table, a working directory, a clock and its
// own variables, so it answers those. The one refusal is the eval depth:
// `%(e.…)` counts the function calls and evals an expansion is inside, and
// this shell's frames are not that count — an `eval` adds to zsh's depth and
// nothing to the frames here, so an answer taken from them would be right
// for a function and wrong for the construct the letter is mostly used with.
// A drawer answers it as nought, which is not a guess: nothing is running
// while a prompt is drawn.
//
// The number is what the condition counts and never the answer; the
// comparison belongs to the condition and is made by the walker. See
// [PromptQuantityResolver].
func (r *Runner) promptQuantity(c PromptCondition, n int) (int, bool) {
	switch c {
	case ConditionExitStatus:
		return r.ExitStatus(), true
	case ConditionJobs:
		live := 0
		for _, j := range r.Jobs() {
			if !j.Finished() {
				live++
			}
		}
		return live, true
	case ConditionEffectiveUser:
		// A read of the process's identity, which is the class .golangci.yml
		// blesses beside `$$` and `$UID`: nothing a script does changes it,
		// and two Runners in one program genuinely share it.
		return os.Geteuid(), true
	case ConditionEffectiveGroup:
		return os.Getegid(), true
	case ConditionPrivileged:
		// Root, which is the reading every measurement taken as an ordinary
		// user agrees with and none of them can tell from another: `%(!.…)`
		// answered `F` with the shell's own privileged option both off and
		// on, so it is not that option, and it ignores the count where
		// `%(#.…)` compares against it. What is left is the identity, and
		// this shell reads it the way FieldPrivilege does.
		if os.Geteuid() == 0 {
			return 1, true
		}
		return 0, true
	case ConditionShellLevel:
		return promptNumber(r.promptVar("SHLVL")), true
	case ConditionSeconds:
		return promptNumber(r.promptVar("SECONDS")), true
	case ConditionLineWidth:
		// The shell's own COLUMNS rather than the terminal's, which is what
		// makes powerlevel10k's prompt-length routine work at all: it sets
		// `local -i COLUMNS=1024` and measures inside that.
		return promptNumber(r.promptVar("COLUMNS")), true
	case ConditionOpenConstructs:
		// Nothing is open: a script that reached an expansion has parsed.
		// The same answer FieldOpenState gives, and measured the same way —
		// every count above nought answers `F` in a script.
		return 0, true
	case ConditionPromptArrayCount:
		elems, _ := r.GetArray(promptArray)
		return len(elems), true
	case ConditionPromptArrayElement:
		elems, _ := r.GetArray(promptArray)
		if n < 0 {
			n = -n
		}
		if n == 0 {
			// A count of nought names the first element, measured: with
			// `psvar=(a b '')` a bare `%(V.…)` answers `T` and it answers
			// `F` with the array empty.
			n = 1
		}
		if n > len(elems) || elems[n-1] == "" {
			return 0, true
		}
		return 1, true
	case ConditionCwdComponents:
		return pathComponents(r.promptVar("PWD")), true
	case ConditionCwdComponentsHome:
		return pathComponents(abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME"))), true
	}
	if v, ok := promptClockQuantity(c, r.Now()); ok {
		return v, true
	}
	return 0, false
}

// promptArray is the name of the array `%(v.…)` and `%(V.…)` ask about.
//
// A constant here rather than a field on the style because it is the one
// thing about these two conditions that is not the shell's state: the array
// is *named* in the dialect that has the letters, and a dialect without them
// never reaches this. It is spelled the way the shell that has it spells it,
// and the tie between that name and its scalar is the dialect's own.
const promptArray = "psvar"

// promptClockQuantity is the conditions drawn from the clock, through
// Runner.Now so that an embedder who pinned the clock pinned these too.
//
// Measured on the tenth of September 2026 at 11:09 on a Thursday: the month
// answered a count of 8 and not 9, which is the months already gone rather
// than the month's number, and the day of the week answered 4 with Sunday as
// nought.
func promptClockQuantity(c PromptCondition, now time.Time) (int, bool) {
	switch c {
	case ConditionMonth:
		return int(now.Month()) - 1, true
	case ConditionDayOfMonth:
		return now.Day(), true
	case ConditionHour:
		return now.Hour(), true
	case ConditionMinute:
		return now.Minute(), true
	case ConditionDayOfWeek:
		return int(now.Weekday()), true
	}
	return 0, false
}

// promptNumber reads one of the shell's variables as a count, and nothing at
// all as nought.
//
// Lenient on purpose: `COLUMNS` and `SECONDS` are the shell's to assign and a
// script may have put anything in them. Measured, zsh reads a `COLUMNS` that
// is not a number as nought — `COLUMNS=abc` behaves exactly as `COLUMNS=0`
// does.
func promptNumber(v string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(v))
	return n
}

// pathComponents counts the parts of a path, with the home directory's `~`
// as one of them.
//
// Measured: `/` has none, `/tmp/a/b/c` has four, `/Users/bhamilton` has two
// and `~` on its own has one — so the marker is a component where the
// leading slash is not, which is the same asymmetry trailingComponents was
// measured to have.
func pathComponents(dir string) int {
	n := 0
	for _, part := range strings.Split(dir, "/") {
		if part != "" {
			n++
		}
	}
	return n
}

// promptClockField is the codes drawn from the clock, through Runner.Now so
// that an embedder who pinned the clock pinned these too.
//
// The braces after a Formats code are a `strftime` format and replace the
// shape the code would otherwise draw: measured through a pty, `%D` is
// `26-09-07`, `%D{%H:%M}` is `04:25` and `%D{}` is nothing — so the braces
// being *there* is the question and not what is in them.
func (r *Runner) promptClockField(f PromptField, arg string, braced bool) (string, bool) {
	layout := ""
	switch f {
	case FieldTime24:
		layout = "15:04:05"
	case FieldTime12:
		layout = "03:04:05"
	case FieldTime24HM:
		layout = "15:04"
	case FieldTime12AMPM:
		layout = "03:04 PM"
	case FieldTime12Padded, FieldTime24Unpadded, FieldTime24HMUnpadded:
	case FieldDate:
		layout = "Mon Jan 02"
	case FieldDateShort:
		layout = "Mon 2"
	case FieldDateMonthDayYear:
		layout = "01/02/06"
	case FieldDateYearMonthDay:
		layout = "06-01-02"
	default:
		return "", false
	}
	now := r.Now()
	if braced {
		// The braces replace the shape the code would otherwise draw, and an
		// empty format is a format: measured, `%D` is `26-09-07` and `%D{}`
		// is nothing at all, which is what strftime of an empty format
		// answers anyway.
		return strftime(arg, now), true
	}
	switch f {
	case FieldTime12Padded:
		return padHourWithASpace(now.Format("3:04PM")), true
	case FieldTime24Unpadded:
		return strings.TrimPrefix(now.Format("15:04:05"), "0"), true
	case FieldTime24HMUnpadded:
		return strings.TrimPrefix(now.Format("15:04"), "0"), true
	}
	return now.Format(layout), true
}

// padHourWithASpace is what zsh's `%t` does to an hour below ten: a space
// rather than a zero, so nine o'clock is " 9:56PM" and ten is "10:08PM".
// Go's layouts have no space-padded hour — `_` pads a day and nothing else.
func padHourWithASpace(clock string) string {
	if len(clock) > 1 && clock[1] == ':' {
		return " " + clock
	}
	return clock
}

// promptVar reads one of the shell's own variables, and the shell's rather
// than the process's: a session that assigned PWD or HOME meant it, and
// asking the operating system instead would answer about a different shell.
func (r *Runner) promptVar(name string) string {
	if v, ok := r.GetVar(name); ok {
		return v
	}
	return ""
}

// lastPathComponent is the final component of a path, and nothing at all for
// no path — where path.Base answers `.`, which is a directory and not the
// absence of one.
func lastPathComponent(dir string) string {
	if dir == "" {
		return ""
	}
	return path.Base(dir)
}

// abbreviateHome writes the home directory as `~`.
//
// The home directory exactly, or a path inside it — not any path that merely
// starts with the same letters, which is why the boundary is checked.
func abbreviateHome(dir, home string) string {
	if home == "" || dir == "" {
		return dir
	}
	if dir == home {
		return "~"
	}
	if strings.HasPrefix(dir, home) && (strings.HasSuffix(home, "/") || dir[len(home)] == '/') {
		return "~" + dir[len(home):]
	}
	return dir
}

// countedComponents is the whole of what a count in front of a path code
// means: from the right for a positive one and from the left for a negative
// one.
//
// Measured on zsh 5.9.2, 2026-09-12, in `/tmp/a/b/c` and in a directory three
// levels under a home. `%2~` is `b/c` and `%-2~` is `/tmp/a` — trailing and
// leading halves of the same path — and a bare minus is minus one, which is
// promptCount's reading and the same one the conditional already took.
//
// whenNought is what a count of nought means, and it is the *only* thing that
// separates the two families of path code. `%~` and `%d` read it as no limit;
// `%c`, `%C` and `%.` read it as one, so `%c` is the last component and `%0c`
// is the same component again. Measured both ways round, including the
// spellings that reach nought sideways: `%-0c` is the trailing component and
// not the leading one.
//
// The minus was refused by name everywhere but the conditional until #1699, on
// the grounds that a plausible wrong answer is worse than a gap. It is not a
// gap any more, so the refusal has nothing left to protect.
func countedComponents(path, arg string, whenNought int) string {
	n := promptCount(arg)
	if n == 0 {
		n = whenNought
	}
	switch {
	case n < 0:
		return leadingComponents(path, -n)
	case n == 0:
		return path
	default:
		return trailingComponents(path, n)
	}
}

// leadingComponents keeps the first n components of a path, which is what a
// *negative* count in front of a path code asks for.
//
// The mirror of trailingComponents and measured the same way, in `/tmp/a/b/c`
// and in `~/tmpprobe/x/y` on zsh 5.9.2, 2026-09-12. Two rules, and the second
// is the asymmetry this shares with its sibling rather than one of its own:
//
//   - `n` counts components from the left, with the leading marker kept:
//     `%-1~` of `/tmp/a/b/c` is `/tmp` and `%-2~` is `/tmp/a`.
//   - **The tilde is a unit and the slash is not.** `%-1~` of `~/tmpprobe/x/y`
//     is `~` alone, where `%-1d` of the same directory spelled out is
//     `/Users` — the first *segment* with its slash in front of it. So a home
//     path of three segments is four units and an absolute one of five
//     segments is five, and `n` at or past that count is the whole path.
func leadingComponents(path string, n int) string {
	if path == "" || n <= 0 {
		return path
	}
	lead, rest := pathLeader(path)
	if rest == "" {
		return path
	}
	parts := strings.Split(rest, "/")
	if lead == "~" {
		// The tilde is the first unit, so `%-1~` is the marker on its own and
		// the segments start at two.
		if n > len(parts) {
			return path
		}
		if n == 1 {
			return lead
		}
		return lead + "/" + strings.Join(parts[:n-1], "/")
	}
	if n >= len(parts) {
		return path
	}
	return lead + strings.Join(parts[:n], "/")
}

// pathLeader splits a path's leading marker off the segments after it.
//
// One reader for both directions of the count, because the marker's two
// readings are exactly what the two of them share: a `~` is a unit and a `/`
// is not, and a helper each is how one of them would come to disagree.
func pathLeader(path string) (lead, rest string) {
	switch {
	case strings.HasPrefix(path, "~"):
		return "~", strings.TrimPrefix(path[1:], "/")
	case strings.HasPrefix(path, "/"):
		return "/", path[1:]
	}
	return "", path
}

// trailingComponents keeps the last n components of a path, where n is the
// numeric argument a prompt code was written with.
//
// Measured on zsh 5.9.2, 2026-09-09, in a ten-segment directory. Three rules,
// and the second is the one worth writing down because it is not what
// "keep the last n" alone would do:
//
//   - `n` counts components from the right: `%2~` of `…/a/b/c/d` is `c/d`.
//   - **`n` at or past the count is the whole path, with its leading marker
//     back.** `%9d` of a ten-segment path is nine segments and no leading
//     `/`; `%10d` of the same path is all ten *with* it. A shell that only
//     joined the last n would answer the tenth case without the slash, which
//     is a plausible path to the wrong place rather than a visible failure.
//     The tilde is one of the units on the abbreviated side, so `%9~` of a
//     six-unit path is the whole thing, tilde included.
//   - `0`, and no argument at all, mean no limit — which countedComponents
//     answers before this is called, since `%c` reads nought as one instead.
func trailingComponents(path string, n int) string {
	if path == "" || n <= 0 {
		return path
	}
	// The leading marker is not a component to be counted from the right, but
	// it is one of the units that decides whether the whole path is asked
	// for: `/` and `~` each stand for a level above the segments after them.
	lead, rest := pathLeader(path)
	if rest == "" {
		return path
	}
	parts := strings.Split(rest, "/")
	if lead == "~" {
		// The tilde is a unit of its own, so a path of five segments under a
		// home is six things and `%6~` is all of it.
		if n > len(parts) {
			return path
		}
	} else if n >= len(parts) {
		return path
	}
	return strings.Join(parts[len(parts)-n:], "/")
}
