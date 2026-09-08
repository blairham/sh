// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The two `q` modifiers: `q-`, minimal quoting, and `q+`, the extended form
// of it. The value comes back in the shortest spelling that reads back as
// itself, which for most values is the value written bare.
//
// Neither `-` nor `+` is a flag of its own here. They are the characters the
// group's `q` eats, and the parser settles that reading once and hands the
// eaten character over in QuoteModifier — so every `-` this package sees in
// Flags is the signed-numeric sort flag, and every `+` is a `+` that no `q`
// could take. See syntax.scanParamFlags for the grammar and the panel behind
// it; the measurements that separate the two readings, on zsh 5.9.2,
// 2026-09-08, over `b=(-1 -10 -3 2 10)`:
//
//	${(o)b}      -1 -10 -3 10 2   lexical, the sort `-` is not asked for
//	${(o-)b}     -10 -3 -1 2 10   signed, so a lone `-` is a sort flag
//	${(oq-)b}    -1 -10 -3 10 2   lexical: the `q` ate the `-`
//	${(oq--)b}   -10 -3 -1 2 10   signed: the *second* `-` is the sort flag
//	${(oq+-)b}   -10 -3 -1 2 10   signed: a `q+` had already taken the slot
//	${(qU-)v}    A\ B             adjacency is literal, not associative
//	${(q-U)v}    'A B'            and the letter behind it changes nothing
//
// What `q-` does, measured one ASCII byte at a time through `a<byte>b`: the
// value is cut at each `'`, each piece is written in single quotes if any
// byte in *that piece* needs quoting and bare if none does, and each `'` is
// written as `\'` outside the quotes. That is why `a'b c` is `a\''b c'` —
// the `a` is bare, the quote is a backslash pair, and the run after it is
// quoted whole because of the space. An empty value is `''`.
//
// `q+` is the same walk with two changes and one whole-value escape hatch:
//
//  1. The quoting decision is taken over the **whole value** rather than
//     per run, so `has'quote` is `'has'\''quote'` where `q-` gives
//     `has\'quote` — neither run needs quotes on its own and both get them.
//  2. The two start-only specials are special **wherever they stand**, so
//     `a~b` is `'a~b'` and `PATH=/x` is `'PATH=/x'` where `q-` leaves both
//     bare. Which is the same table asked with the position pinned to zero.
//  3. One byte that cannot be written as itself moves the **whole value**
//     into `$'…'`: `a<tab>b` is `$'a\tb'` and `a\001b` is `$'a\C-Ab'`,
//     where `q-` puts the first in quotes and leaves the second raw.
//
// Both are whole-word rewrites and not per-character ones, which matters to
// exactly one composition: a `(~)`-marked join separator is refused beside
// either, the way it is beside `(qq)` and up. See interp/tildeflaggroup.go.

// quotableSpecials are the bytes a word cannot carry bare. Measured through
// `${(q-)…}` one byte at a time: every other ASCII byte, every control byte
// but tab and newline, and every byte above 0x7f comes back unquoted, so a
// value with an escape or a `\377` in it and nothing else is written as
// itself.
//
// `'` is in the set for the sake of `${(q)…}`, which escapes it. Minimal
// quoting never asks about one, having cut the value at every `'` first —
// but the *extended* form does ask, and needs the answer to be yes: a value
// with a quote in it is quoted throughout, `has'quote` coming back
// `'has'\”quote'` rather than bare.
const quotableSpecials = " \t\n`$\"'\\*?[](){}<>|;&#^"

// quotableAtStart are the two bytes that need quoting only as a word's first
// byte, where one starts a tilde expansion and the other an equals one.
// Measured, and the same answer for all three constructs that quote by
// position: `${(q)…}`, `${(q-)…}` and the `:q` modifier leave `a~b` and
// `PATH=/x` alone, where `~x` is `\~x` and `=x` is `\=x`.
//
// `${(q+)…}` is the fourth reader of this table and the one that does not
// read the position: measured, it quotes `a~b` and `PATH=/x` too.
const quotableAtStart = "~="

// quotableByte reports whether the byte at offset i of a value has to be
// quoted for the value to read back as itself.
func quotableByte(i int, c byte) bool {
	if strings.IndexByte(quotableAtStart, c) >= 0 {
		return i == 0
	}
	return strings.IndexByte(quotableSpecials, c) >= 0
}

// quoteInRuns is the shape both modifiers write: the value cut at each `'`,
// every quote written `\'` outside any quoting, and every non-empty run
// wrapped in single quotes when needsQuotes says so. run is the piece and at
// its offset in the whole value.
//
// One walk for both, because the two differ only in that predicate. A second
// copy of the walk is how they would come to disagree about the empty run at
// the end of `x'`, which neither writes and only one of them would have been
// tested on.
func quoteInRuns(v string, needsQuotes func(run string, at int) bool) string {
	if v == "" {
		// The one value with nothing to quote that still cannot be written
		// bare, since bare it is no word at all.
		return "''"
	}
	var b strings.Builder
	for at := 0; ; {
		rest := v[at:]
		end := strings.IndexByte(rest, '\'')
		run := rest
		if end >= 0 {
			run = rest[:end]
		}
		if run != "" {
			// An empty run is written as nothing rather than as `''`:
			// measured, `x'` is `'x'\'` and `a''b` is `'a'\'\''b'`.
			if needsQuotes(run, at) {
				b.WriteByte('\'')
				b.WriteString(run)
				b.WriteByte('\'')
			} else {
				b.WriteString(run)
			}
		}
		if end < 0 {
			return b.String()
		}
		b.WriteString(`\'`)
		at += end + 1
	}
}

// quoteMinimal is the `q-` style: single quotes only around the runs that
// need them.
//
// The offset handed to quotableByte is the run's offset in the whole value
// rather than zero, because the two start-only specials are special at the
// *value's* start and not at each run's: measured, `'~x` is `\'~x`, the `~`
// needing nothing a byte into the word, where `~'a` is `'~'\'a`.
func quoteMinimal(v string) string {
	return quoteInRuns(v, func(run string, at int) bool {
		for i := range len(run) {
			if quotableByte(at+i, run[i]) {
				return true
			}
		}
		return false
	})
}

// quoteExtended is the `q+` style. See the three differences above.
func quoteExtended(v string) string {
	if v == "" {
		return "''"
	}
	if extendedRenders(v) {
		return quoteExtendedRendered(v)
	}
	// One decision for the whole value, and the position pinned to zero so
	// that a `~` or an `=` counts wherever it stands.
	quote := false
	eachQuotableByte(v, func(_ int, c byte) {
		if quotableByte(0, c) {
			quote = true
		}
	}, func(int, string) {})
	return quoteInRuns(v, func(string, int) bool { return quote })
}

// extendedRendered reports whether `q+` writes this byte as an escape rather
// than as itself — which is also the question of whether the *whole* value
// moves into `$'…'`, the flag being all-or-nothing about it: measured,
// `a\001b c` is `$'a\C-Ab c'`, with the space that would have asked for
// quotes written as itself inside the escape instead.
//
// The bytes are the control ones, delete, and any byte that is not part of a
// valid multibyte rune — the same three-way split every other quoting flag
// here reads, which is why the caller hands them over through
// eachQuotableByte rather than walking the string itself. A rune passes
// through: measured, `café` is `café` and `café x` is `'café x'`.
//
// Whether a byte above 0x7f is one of these is a question about the locale
// and not about the flag, and this package answers it the one way it answers
// it everywhere: measured under `LC_ALL=C.UTF-8`, `${(q+)…}` on `a\377b` is
// `$'a\M-\C-?b'` and `${(q)…}` is `a$'\377'b`, while under `LC_ALL=C` both
// leave the byte as itself. The tree spells the first reading, for `q` and
// `qqqq` alike, so `q+` spells it too — a second opinion about UTF-8 in the
// same file is worse than a wrong one.
func extendedRendered(c byte) bool {
	return c < 0x20 || c >= 0x7f
}

// extendedRenders reports whether any byte of the value is one of those.
func extendedRenders(v string) bool {
	found := false
	eachQuotableByte(v, func(_ int, c byte) {
		if extendedRendered(c) {
			found = true
		}
	}, func(int, string) {})
	return found
}

// quoteExtendedRendered is the `$'…'` spelling the whole value moves into.
//
// The vocabulary is the caret one and not `qqqq`'s octal, which is what makes
// `q+` a different flag rather than a wider table. Measured on zsh 5.9.2,
// 2026-09-08, byte by byte through `a<byte>b`:
//
//	\t \n              the only two named escapes — 7 is `\C-G`, not `\a`,
//	                   and 27 is `\C-[`, not `\E`
//	\C-X               every other byte below 0x20, and 0x7f as `\C-?`
//	\M-<X>             a byte above 0x7f: the meta prefix, then the low
//	                   seven bits spelled by the same three rules —
//	                   0x80 is `\M-\C-@`, 0x89 is `\M-\t`, 0xa0 is `\M- `
//	\' \\              a quote and a backslash, which the quotes cannot hold
//	                   — `!` is *not* escaped here, where `qqqq` writes `\!`
//
// Two bytes are spelled by that shell in a way that does not read back:
// 0xa7 comes out `\M-'` and 0xdc `\M-\`, each ending the `$'…'` early.
// Measured and reproduced rather than corrected, because the record is of
// what the shell does; they are the two values left out of the round-trip
// property in interp/minimalquote_test.go, and the reason is written there.
func quoteExtendedRendered(v string) string {
	var b strings.Builder
	b.WriteString("$'")
	eachQuotableByte(v, func(_ int, c byte) {
		switch {
		case c == '\'':
			b.WriteString(`\'`)
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x80:
			b.WriteString(`\M-` + extendedByteEscape(c-0x80))
		default:
			b.WriteString(extendedByteEscape(c))
		}
	}, func(_ int, raw string) { b.WriteString(raw) })
	b.WriteByte('\'')
	return b.String()
}

// extendedByteEscape spells one byte inside that `$'…'`: the caret notation
// for a control byte, and the byte itself for anything else.
//
// The caret vocabulary is not written out a second time here. It is
// controlEscaped's, under ControlEscapeCaret — the same shell's answer to the
// same question, measured first for the alias listing — and a second copy is
// how the two would come to disagree about which bytes have a name.
func extendedByteEscape(c byte) string {
	if extendedRendered(c) {
		return controlEscaped(ControlEscapeCaret, c)
	}
	return string(c)
}

// extendedQuoteRefusal names the one legal spelling of `q+` this interpreter
// does not carry: a `q+` with a further `q` in the group.
//
// That shell reads `${(q+q)v}` and `${(q+Uq)v)` rather than erroring — the
// asymmetry with `q-`, which refuses both — and answers them with a spelling
// that does not read back. Measured 2026-09-08 on `u="has'quote"`:
//
//	${(qq)u}     'has'\''quote'   the doubled style, which reads back
//	${(q+q)u}    'has'quote'      the quote written raw, which does not
//	${(q+qq)u}   'has'quote'      and the count past two changes nothing
//
// So the group is refused by name rather than reproduced. A shell that
// hands back a word it cannot read is not a behavior to model; a shell that
// says it will not is one this reader can act on.
func extendedQuoteRefusal(e *syntax.ParamExpr) bool {
	return e != nil && e.QuoteModifier == '+' && strings.Count(e.Flags, "q") > 1
}

// signedSortFlag reports whether the group asked for the signed-numeric sort.
//
// Every `-` left in Flags is one, the parser having taken the modifier out,
// so this is a containment test and not a disambiguation.
func signedSortFlag(e *syntax.ParamExpr) bool {
	return e != nil && strings.ContainsRune(e.Flags, '-')
}
