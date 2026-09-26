// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// Where one column puts a substitution refused in a **here-document body**,
// and what it writes under it.
//
// That column places the refusal at the line *after* the here-document rather
// than at the line the body holds the substitution on, and it writes a plain
// sentence under it rather than the quote or the open-context line every
// other position gets. Both messages take the same line.
//
// Measured 2026-09-26 over script files under `env -i PATH=/usr/bin:/bin
// HOME=<scratch> LC_ALL=C` with stdin from /dev/null, against
// `/opt/homebrew/bin/zsh -f` 5.9.2 (`not a Go executable` by `go version
// -m`), each body holding `$(echo hi; for)`:
//
//	program                                       reference     before
//	cat <<END ⏎ <body> ⏎ END ⏎                     :4: twice     :2: then :3:
//	the same with `echo after` under it            :4: twice     :2: then :3:
//	echo p ⏎ cat <<END ⏎ <body> ⏎ END ⏎            :5: twice     :3: then :4:
//	cat <<END ⏎ x ⏎ <body> ⏎ END ⏎                 :5: twice     :3: then :4:
//	cat <<END ⏎ <body> ⏎ END  (no final newline)   :3: twice     :2: then :3:
//	cat <<END | cat ⏎ <body> ⏎ END ⏎ printf        :4: twice     :2: then :3:
//	{ cat; } <<END ⏎ <body> ⏎ END ⏎ echo a         :4: twice     :2: then :3:
//
// So it is the **delimiter's line**, plus one where a line follows it. The
// row with no final newline is what says the `+1` is a rule and not a
// constant: with the delimiter as the last line of the file the number stops
// there. It is the rule [Runner.substWordEcho] already applies to the second
// message — a newline ends the line and the reader has taken it — and here it
// reaches the first message too. It does not move with the file's length,
// with the command's shape, or with where in the body the substitution is
// written.
//
// # And the sentence under it
//
//	program                            reference second line   before
//	cat <<END ⏎ $(echo hi; for) ⏎ END   parse error             unmatched "
//	echo $(echo hi; for)                parse error near `…'    identical
//	echo "$(echo hi; for)"              unmatched "             identical
//	cat <<<"$(echo hi; for)"            unmatched "             identical
//
// The three controls are byte-identical, so the machinery is right in general
// and a here-document body is the one input it answered wrongly — and the
// wrong answer is visible in the wording it chose. A body is lexed the way a
// double-quoted string is, so the failing span arrives quoted and the level
// records an open `"` that the program does not contain. The sentence is the
// dialect's own, written in place of the levels rather than beside them,
// which is also why the quoting never has to be unpicked here.

// heredocBodyRefusalLocation is that line, and whether this dialect uses it.
//
// The here-**string** is excluded because [Runner.inHeredocBody] is what is
// read, and the older substitution spelling because its body is read with the
// script's line — the two exclusions every reader of
// [Runner.expansionBodyLine] makes.
func (r *Runner) heredocBodyRefusalLocation(span syntax.Span) (int, bool) {
	if span.Backquoted || !r.inHeredocBody || r.expansionBodyLine == 0 {
		return 0, false
	}
	if !r.diag().HeredocBodyRefusalIsLocatedAfterTheDelimiter {
		return 0, false
	}
	// The delimiter stands on the line past the body's last, and the body's
	// text is what says how many lines that is.
	at := r.expansionBodyLine + strings.Count(r.heredocBodyText, "\n")
	text, base := r.textInForce()
	if text == "" || at-base < len(strings.Split(text, "\n")) {
		// A newline ended the delimiter's line and the reader took it.
		//
		// Assumed where nobody said what the program's text is, which is an
		// embedder that has not called [Runner.SetProgramText] rather than
		// any route this tree ships: a program whose very last byte is its
		// here-document delimiter is the one shape the assumption is wrong
		// for, and reporting the delimiter's line for every ordinary
		// program would be wrong for all the rest.
		at++
	}
	return at, true
}

// heredocBodyRefusalSentence is what that dialect writes under the refusal in
// place of the quote and the open-context lines, or the empty string where it
// writes none.
func (r *Runner) heredocBodyRefusalSentence(span syntax.Span) (string, int, bool) {
	at, ok := r.heredocBodyRefusalLocation(span)
	if !ok {
		return "", 0, false
	}
	form := r.diag().HeredocBodyRefusalSentence
	if form == "" {
		return "", 0, false
	}
	return Wording(form, "parse error") + "\n", at, true
}
