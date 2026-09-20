// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// A quotation inside a subscript that arrived already expanded stops the
// expansion it holds, where the dialect reads that subscript as a quoting
// context at all.
//
//	typeset -A m; m[q]=7; kq=q
//	e="m['\$kq']"; printf '%s' "$(( $e ))"
//
// is `0` in bash 5.3.20, ksh93u+ 2012-08-01 and zsh 5.9.2 and was `7` here in
// two of those three columns: the brackets came out of `$e`, so the subscript
// is read a second time, and the `$kq` inside the apostrophes was performed on
// that second reading (#3918).
//
// # The key each column holds, which is what identifies the mechanism
//
// Measured 2026-09-20, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> f.sh` over
// a script file with standard input on the null device, storing through the
// re-read subscript and then listing the table's keys — `typeset -A m; kq=q`,
// then `e='m[<text>]'` and `(( $e = 42 ))`:
//
//	text in the brackets	bash 5.3.20	ksh93u+	zsh 5.9.2
//	`'$kq'`             	`$kq`      	`$kq`  	`'q'`
//	`"$kq"`             	`q`        	`q`    	`"q"`
//	`$kq`               	`q`        	`q`    	`q`
//
// The second and third rows are the controls and they are what make this one
// question rather than two. The same expansion, in the same brackets, reached
// by the same re-read: a double quotation performs it in every column, and an
// apostrophe stops it in two. The third says the re-read itself is not in
// doubt.
//
// **Whose question it is** is already answered. bash and ksh93 read a
// subscript as a quoting context and zsh does not — Semantics.
// SubscriptIsAQuotingContext, measured one spelling over on `${a['q']}` —
// and the columns part here exactly as they part there: where the apostrophes
// are quotation they stop what they hold and are then removed, and where they
// are ordinary characters they stop nothing and stay in the key. So this is
// that axis read at a site that was not asking it, and not an axis of its own:
// a preset answering it `no` and stopping an expansion would be a fourth
// answer no column gives.
//
// # Only the expansion, and only an apostrophe
//
// Whether the quote characters themselves survive into the key is the same
// axis's other half and was already right. That is why bash and ksh93 reach
// `$kq` — three characters, the apostrophes gone — while zsh reaches `'q'`
// with them still there.
//
// A double quotation is passed over rather than handled, because every column
// performs what one holds; it is walked only so that an apostrophe inside one
// is not read as quotation. A backslash is passed over for the same reason
// from the other side: `\$kq` is `$kq` in all three columns already, which is
// the escape the scan below has always honored.
func quotationStopsASubscriptsExpansion(text string) bool {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case syntax.ArithValueMark, '\\':
			// A byte a value carried opens no quotation, and neither does
			// one the text escaped.
			i++
		case '"':
			end := indexUnmarked(text[i+1:], '"')
			if end < 0 {
				return false
			}
			i += end + 1
		case '\'':
			end := indexUnmarked(text[i+1:], '\'')
			if end < 0 {
				// An unterminated quotation stops nothing. It is the same
				// answer subscriptQuoteRemoval gives the same text, and for
				// the same reason: text that ran off the end is not a key
				// anybody wrote on purpose.
				return false
			}
			if syntax.UnmarkedExpansion(text[i+1 : i+1+end]) {
				return true
			}
			i += end + 1
		}
	}
	return false
}

// arithSubscriptKeyQuoted performs the expansions a re-read subscript still
// holds, leaving the ones an apostrophe stops.
//
// What an apostrophe held is written out **marked**, which is the same claim
// the surrounding quotes make: those bytes are characters of the key now and
// nothing may read them as syntax again. The apostrophes themselves are left
// unmarked, because taking them off is the other half of the axis and is
// subscriptQuoteRemoval's to do — so `'$kq'` comes through as `'$kq'` with its
// middle sealed, and the key is the three characters `$kq`.
func (r *Runner) arithSubscriptKeyQuoted(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	run := 0
	flush := func(end int) {
		if end > run {
			b.WriteString(r.arithSubscriptKeyScan(text[run:end]))
		}
	}
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case syntax.ArithValueMark, '\\':
			i++
		case '"':
			end := indexUnmarked(text[i+1:], '"')
			if end < 0 {
				flush(len(text))
				return b.String()
			}
			i += end + 1
		case '\'':
			end := indexUnmarked(text[i+1:], '\'')
			if end < 0 {
				flush(len(text))
				return b.String()
			}
			flush(i)
			b.WriteByte('\'')
			b.WriteString(markArithValue(text[i+1 : i+1+end]))
			b.WriteByte('\'')
			i += end + 1
			run = i + 1
		}
	}
	flush(len(text))
	return b.String()
}
