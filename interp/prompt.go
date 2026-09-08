// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path"
	"strconv"
	"strings"
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

	// Octal says three octal digits after the escape are the byte they name,
	// which is how bash spells a character it has no letter for.
	//
	// Exactly three, measured: `\007` drew the bell and `\101` drew `A`, while
	// `\0`, `\1`, `\10`, `\00` and `\8` were all left as they were written.
	// A value above 255 is taken low byte first — `\400` drew a NUL.
	Octal bool

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
// The rune result is the code the resolver had no answer for, or that the
// table listed as Unsupported, and is meaningful only when ok is false. The
// text returned with it is what had been drawn up to that point, which no
// caller uses and which is returned rather than dropped so that a caller
// wanting to report *where* in the prompt it stopped can.
func ExpandPromptStyle(st PromptStyle, text string, field PromptResolver) (string, rune, bool) {
	if st.Escape == 0 || text == "" {
		return text, 0, true
	}
	var b strings.Builder
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != st.Escape {
			b.WriteRune(runes[i])
			continue
		}
		if i+1 >= len(runes) {
			// An escape with nothing after it. There is no code to look up,
			// so none of the answers below applies — and the two readers
			// measured *differ* here, which is why the table decides: a
			// prompt draws the character itself and the expansion flag drops
			// it. See PromptStyle.TrailingEscapeIsDropped.
			if !st.TrailingEscapeIsDropped {
				b.WriteRune(runes[i])
			}
			break
		}
		code := runes[i+1]
		i++
		if fld, ok := st.Codes[code]; ok {
			arg, braced := "", false
			if st.Formats[code] {
				var next int
				arg, next, braced = promptArgument(runes, i+1)
				i = next
			}
			v, answered := field(fld, arg, braced)
			if !answered {
				return b.String(), code, false
			}
			b.WriteString(v)
			continue
		}
		if seq, ok := st.Sequences[code]; ok {
			b.WriteString(seq)
			continue
		}
		if layer, ok := st.Colors[code]; ok {
			arg, next, _ := promptArgument(runes, i+1)
			i = next
			b.WriteString(colorSequence(layer, arg))
			continue
		}
		if v, ok := promptOctalByte(st.Octal, runes, i); ok {
			b.WriteByte(v)
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
		v, answered := field(FieldNone, string(code), false)
		if !answered {
			return b.String(), code, false
		}
		b.WriteString(v)
	}
	return b.String(), 0, true
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
		if r.promptUser == "" {
			return "", false
		}
		return r.promptUser, true
	case FieldHost:
		host, _, _ := strings.Cut(r.promptHost, ".")
		return host, r.promptHost != ""
	case FieldHostFull:
		return r.promptHost, r.promptHost != ""
	case FieldCwd:
		return abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME")), true
	case FieldCwdFull:
		return r.promptVar("PWD"), true
	case FieldCwdBase:
		return lastPathComponent(abbreviateHome(r.promptVar("PWD"), r.promptVar("HOME"))), true
	case FieldCwdBaseFull:
		return lastPathComponent(r.promptVar("PWD")), true
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
	}
	if v, ok := r.promptClockField(f, arg, braced); ok {
		return v, true
	}
	// The session's own facts, which a Runner has not got: the history
	// number, how many commands this session has run, and the terminal's
	// name. Refused by name rather than answered with a plausible zero.
	return "", false
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
