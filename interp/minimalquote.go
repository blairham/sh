// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The `q-` expansion flag: minimal quoting. The value comes back in the
// shortest spelling that reads back as itself, which for most values is the
// value written bare.
//
// `-` is not a flag of its own. It is a modifier the `q` in front of it eats,
// and every other `-` in a flag group is the signed-numeric sort flag, which
// this interpreter does not carry and refuses by name. The two are told apart
// by adjacency and by count, measured 2026-09-08 on zsh 5.9.2 over
// `b=(-1 -10 -3 2 10)`:
//
//	${(o)b}      -1 -10 -3 10 2   lexical, the sort `-` is not asked for
//	${(o-)b}     -10 -3 -1 2 10   signed, so a lone `-` is a sort flag
//	${(oq-)b}    -1 -10 -3 10 2   lexical: the `q` ate the `-`
//	${(oq--)b}   -10 -3 -1 2 10   signed: the *second* `-` is the sort flag
//	${(oq+-)b}   -10 -3 -1 2 10   signed: a `q+` had already taken the slot
//	${(-q-)b}    minimal quoting  and the modifier may be the later `-`
//
// That last row is zsh's answer and not this one's: the leading `-` is a sort
// flag whether or not it would have changed anything, so the group is refused
// for it. A group is refused for what is written in it rather than for what
// that would have come to on the value in hand, which is the rule `(A)` is
// refused under and for the same reason — a check on whether the flag *fired*
// would carry a line on one run and refuse it on the next.
//
// The adjacency is literal — `${(qU-)v}` on `a b` is `a\ b`, the plain `q`
// style — and only a lone `q` takes the modifier, `${(qq-)v}` and `${(q-q)v}`
// both being errors in the flags rather than a doubled style with a modifier.
//
// What the flag then does, measured one ASCII byte at a time through
// `a<byte>b`: the value is cut at each `'`, each piece is written in single
// quotes if any byte in it needs quoting and bare if none does, and each `'`
// is written as `\'` outside the quotes. That is why `a'b c` is `a\''b c'` —
// the `a` is bare, the quote is a backslash pair, and the run after it is
// quoted whole because of the space. An empty value is `''`.
//
// This is a whole-word rewrite and not a per-character one, which matters to
// exactly one composition: a `(~)`-marked join separator is refused beside it,
// the way it is beside `(qq)` and up. See interp/tildeflaggroup.go.

// quotableSpecials are the bytes a word cannot carry bare. Measured through
// `${(q-)…}` one byte at a time: every other ASCII byte, every control byte
// but tab and newline, and every byte above 0x7f comes back unquoted, so a
// value with an escape or a `\377` in it and nothing else is written as
// itself.
//
// `'` is in the set for the sake of `${(q)…}`, which escapes it. Minimal
// quoting never asks about one, having cut the value at every `'` first.
const quotableSpecials = " \t\n`$\"'\\*?[](){}<>|;&#^"

// quotableAtStart are the two bytes that need quoting only as a word's first
// byte, where one starts a tilde expansion and the other an equals one.
// Measured, and the same answer for all three constructs that quote:
// `${(q)…}`, `${(q-)…}` and the `:q` modifier leave `a~b` and `PATH=/x`
// alone, where `~x` is `\~x` and `=x` is `\=x`.
const quotableAtStart = "~="

// quotableByte reports whether the byte at offset i of a value has to be
// quoted for the value to read back as itself.
func quotableByte(i int, c byte) bool {
	if strings.IndexByte(quotableAtStart, c) >= 0 {
		return i == 0
	}
	return strings.IndexByte(quotableSpecials, c) >= 0
}

// quoteMinimal is the `q-` style: single quotes only around the runs that
// need them.
func quoteMinimal(v string) string {
	if v == "" {
		// The one value with nothing to quote that still cannot be written
		// bare, since bare it is no word at all.
		return "''"
	}
	var b strings.Builder
	for at := 0; ; {
		rest := v[at:]
		end := strings.IndexByte(rest, '\'')
		if end < 0 {
			writeMinimalRun(&b, rest, at)
			return b.String()
		}
		writeMinimalRun(&b, rest[:end], at)
		b.WriteString(`\'`)
		at += end + 1
	}
}

// writeMinimalRun writes one run of a value that has no `'` in it, in single
// quotes when any byte in it needs them and bare when none does.
//
// at is the run's offset in the whole value rather than zero, because the two
// start-only specials are special at the *value's* start and not at each
// run's: measured, `'~x` is `\'~x`, the `~` needing nothing a byte into the
// word, where `~'a` is `'~'\'a`.
func writeMinimalRun(b *strings.Builder, run string, at int) {
	quote := false
	for i := range len(run) {
		if quotableByte(at+i, run[i]) {
			quote = true
			break
		}
	}
	if quote {
		b.WriteByte('\'')
	}
	b.WriteString(run)
	if quote {
		b.WriteByte('\'')
	}
}

// minimalQuoteModifier reports whether the `-` at offset i of a flag group is
// the modifier that turns `q` into minimal quoting, rather than the sort flag
// of the same spelling. See the measurements above.
func minimalQuoteModifier(flags string, i int) bool {
	return i > 0 && flags[i-1] == 'q' && strings.Count(flags, "q") == 1
}
