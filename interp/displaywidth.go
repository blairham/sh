// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"unicode"

	"github.com/blairham/sh/internal/eastasian"
)

// How wide a terminal draws a character, for the two readers in this package
// that ask: the prompt language's `%(l.…)` column — see promptWalk.cell — and
// the `(m)` expansion flag's length, `${(m)#x}`.
//
// One function rather than two copies of the same three lines, because the
// two have to agree. powerlevel10k measures a *component* with `${(m)#…}` and
// measures the *whole prompt* by binary search through `%N(l.…)`, then
// subtracts one from the other to decide how much of a path it can draw; two
// width tables that disagree put a plausible number on both sides of that
// subtraction and no message anywhere.
//
// This is not settled by running a shell, and repl.runeWidth says why at
// length: what is being modeled is the terminal rather than the shell. Three
// classes, measured against zsh 5.9.2 through `${(m)#…}` under a UTF-8 locale:
//
//	${(m)#$'日'}       2   East Asian Width W and F
//	${(m)#$'Ａ'}       2
//	${(m)#$'가'}       2
//	${(m)#$'\U1F600'}  2
//	${(m)#$'é'}  1   a combining mark is no column of its own
//	${(m)#$'҈'}   1   Me as well as Mn
//	${(m)#$'é'}        1   Ambiguous is one column, not two
//	${(m)#$'\x01'}     1   and a control character is one
//	${(m)#$'\e[31m'}   5
//
// **The control row is where this parts company with the line editor**, which
// counts a control character as no cell at all — a terminal draws nothing for
// it — and is why repl.runeWidth is a third reader rather than this one. The
// prompt language agrees with `(m)` here and not with the editor, which is
// measured and is the whole reason the shared thing is the wide table rather
// than a width. See the package comment on internal/eastasian.
//
// Two measured divergences, recorded rather than chased. Both are that shell
// reading its platform's `wcwidth` and neither is a rule that can be pointed
// at in the standard:
//
//   - The format characters, Cf. Zero-width space, soft hyphen and the
//     left-to-right mark measure 0 there and 1 here, while the word joiner
//     U+2060 and the Arabic number sign U+0600 — Cf as well — measure 1 in
//     both. A table that split its own class that way would be picking a side
//     in somebody else's disagreement, which is the line repl.runeWidth draws
//     and this follows.
//   - The regional indicators, U+1F1E6…U+1F1FF, measure 2 there and 1 here.
//     East Asian Width calls them Neutral; that platform's table does not.
func displayWidth(r rune) int {
	if unicode.In(r, unicode.Mn, unicode.Me) {
		return 0
	}
	if eastasian.Wide(r) {
		return 2
	}
	return 1
}

// displayColumns is how wide a terminal draws a whole string.
//
// A byte that begins no valid sequence is one character of one byte and one
// column, which is what DecodeRuneInString's replacement rune measures — the
// same reading characterCount gives it, so a value this shell cannot decode
// is the same length under `${#x}` and under `${(m)#x}`.
func displayColumns(v string) int {
	n := 0
	for _, r := range v {
		n += displayWidth(r)
	}
	return n
}
