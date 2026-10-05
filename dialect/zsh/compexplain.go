// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strconv"

	"github.com/blairham/sh/interp"
)

// What a `compadd -X` explanation or `-x` message draws.
//
// The manual gives an explanation a small part of the prompt language: `%B`,
// `%S`, `%U`, `%F`, `%K` and their lower-case partners, `%{…%}`, and `%%` for
// a percent sign. It matters because the `format` style is where a
// configuration spells its headings — `' %F{yellow}-- %d --%f'` is the common
// shape — and `_description` hands that through to here. Drawn literally,
// every block of every listing carried `%F{yellow}` over it (#6146).
//
// Measured on zsh 5.9.2, 2026-10-05, through a pseudo-terminal with a
// `.list-choices` widget of my own, so that what is read is this builtin and
// not a shipped function:
//
//	-X ' %F{yellow}-- %n %d x %% --%f%B b%b%{Q%}%1F' -J g alpha beta
//	   ' \e[33m-- 2  x % --\e[39m\e[1m b\e[0mQ\e[31m'
//	-X '[n=%n][d=%d][3n=%3n][q=%q][N=%N]' -J g1 alpha beta gamma
//	   [n=3][d=][3n=3][q=][N=]
//	-X 'E[%E]G[%G]C[%(?.t.f)]D[%D{%Y}]T[%3<..<abcdef]P[%]'
//	   E[]G[]C[?.t.f)]D[{}]T[..<abcdef]P[
//
// So:
//
//   - **`%n` is a count and every other letter is nothing.** The count is how
//     many matches *this call* added, duplicates included — candidates
//     `alpha zzz alright alpha` against `al` draw 3 — and a count in front of
//     it changes nothing. In a `-x` message it is -1, with or without matches.
//   - **The rest of the prompt language is not here.** `%(`, `%D`, `%<` and
//     `%E` are unknown codes like any other, dropped with their letter, so the
//     text a conditional or a format would have read is drawn as it stands.
//   - **A restore writes back boldface, standout and underline, never a
//     color**, and `%B` restores nothing at all. See
//     interp.PromptStyle.RestoreLeavesColors for the pair of rows that part
//     this from a prompt.
//
// One walker, interp's, with a table of its own: the sequences and colors are
// the prompt's, so a terminal description that changes `%S` changes it here
// too, and under `TERM=dumb` the attribute codes write nothing in both.

// explanationStyle is the part of the prompt table an explanation reads.
func explanationStyle() interp.PromptStyle {
	prompt := PromptStyle()
	keep := func(codes string) map[rune]string {
		out := map[rune]string{}
		for _, c := range codes {
			if v, ok := prompt.Sequences[c]; ok {
				out[c] = v
			}
		}
		return out
	}
	caps := map[rune]string{}
	for c, name := range prompt.SequenceCapabilities {
		if c != 'E' {
			caps[c] = name
		}
	}
	return interp.PromptStyle{
		Escape:                  '%',
		NumericArgument:         true,
		TrailingEscapeIsDropped: true,
		RestoreLeavesColors:     true,
		Unknown:                 interp.DropBoth,
		Codes: map[rune]interp.PromptField{
			'%': interp.FieldEscape,
			'{': interp.FieldNonPrintingStart,
			'}': interp.FieldNonPrintingEnd,
			// The letter that is the user's name in a prompt is the count
			// here; explanationField answers it. Reusing the prompt's field
			// rather than inventing one keeps the walker's table closed.
			'n': interp.FieldUser,
		},
		Sequences:            keep("BbUuSsfk"),
		SequenceCapabilities: caps,
		Visual: map[rune]interp.PromptVisual{
			// Measured: `%U%S%Ba` writes `\e[1m` alone where a prompt writes
			// the standout and the underline back after it.
			'B': {Attribute: interp.AttributeBold},
			'b': {Attribute: interp.AttributeBold, Off: true, Restores: true},
			'U': {Attribute: interp.AttributeUnderline},
			'u': {Attribute: interp.AttributeUnderline, Off: true, Restores: true},
			'S': {Attribute: interp.AttributeStandout},
			's': {Attribute: interp.AttributeStandout, Off: true, Restores: true},
			'f': {Attribute: interp.AttributeForeground, Off: true},
			'k': {Attribute: interp.AttributeBackground, Off: true},
		},
		Colors: prompt.Colors,
	}
}

// explanationText is an explanation as the listing draws it, with count as
// its `%n`.
//
// The terminal's own strings and a 24-bit color's nearest palette entry are
// the runner's answers, so they are asked of it; nothing else in an
// explanation is about the shell.
func explanationText(r *interp.Runner, text string, count int) string {
	field := func(f interp.PromptField, arg string, braced bool) (string, bool) {
		switch f {
		case interp.FieldUser:
			return strconv.Itoa(count), true
		case interp.FieldEscape:
			return "%", true
		case interp.FieldTerminalCapability, interp.FieldNearestColor:
			return r.PromptField(f, arg, braced)
		}
		// The non-printing markers draw nothing, and an unknown code is
		// dropped with its letter.
		return "", true
	}
	out, _, _ := interp.ExpandPromptStyle(explanationStyle(), text, field, nil)
	return out
}
