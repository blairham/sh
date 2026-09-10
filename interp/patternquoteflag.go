// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"strings"

	"github.com/blairham/sh/syntax"
)

// The `(b)` expansion flag: backslash-escape a value's pattern
// metacharacters so that the text can be pasted into a pattern and match
// itself.
//
// It is the third quoting flag beside `(q)` and `(Q)`, and the one whose
// question is about *patterns* rather than about shell syntax. That is what
// separates it from `(q)`, and a space is the character that says so:
// measured on zsh 5.9.2 with `v='a b*c?d[e]'`, `${(b)v}` is `a b\*c\?d\[e\]`
// and `${(q)v}` is `a\ b\*c\?d\[e\]`. An empty value is the second
// difference — empty here and `''` there, backslashes having no way to spell
// an empty *shell word* while an empty *pattern* is simply empty.
//
// The one thing it is for is the round trip, and it holds for every value:
// `[[ $v == ${~${(b)v}} ]]` is true whatever `v` holds. The tilde is part of
// the statement rather than an aside — a value's metacharacters are not live
// in a pattern until something asks for them, so the flag's product is text
// destined for a pattern *reader*, and the workload writes it straight into
// one. Without the tilde the round trip fails for every value the flag
// touches, the backslashes being ordinary characters to a literal match.

// patternQuoteMeta is the set `(b)` escapes, measured byte by byte over the
// whole of 1..127 on zsh 5.9.2 (Homebrew, arm64), 2026-09-10:
//
//	# ( ) * < > ? [ \ ] ^ | ~
//
// Everything else is written through as itself — including the space, the
// tab, the newline, `!`, `-`, `{`, `}`, `$`, `%`, `;`, `&`, the three
// quote characters, the backquote, every other control byte, and every byte
// above 0x7f, so multibyte UTF-8 and invalid bytes alike pass through
// untouched.
//
// **The set does not move with the glob options**, which is measured rather
// than assumed: the same 127-row table comes back identical under
// `extendedglob`, under `kshglob`, with `bareglobqual` unset, with `noglob`
// set, and under every combination of those. So `#` and `^` are escaped by
// this flag even in the shell state where they are ordinary text — which
// costs nothing, an escaped ordinary character being that character, and is
// the only answer that can be right for text whose reader's options are not
// this expansion's to know.
//
// Nor does it move with position. `~` is escaped in the middle of a value as
// well as at its start, which is where it parts company with `(q)`: measured,
// `${(q)v}` on `a~b` is `a~b` and `${(b)v}` on the same value is `a\~b`. A
// pattern's `~` is the exclusion operator wherever it stands, where a shell
// word's is a tilde expansion only at the front.
//
// It is patternMeta and extendedPatternMeta with the two closing brackets
// added — `>` for the numeric range the dialect opens with `<`, and `]` for
// the bracket expression it opens with `[`. bracketMeta's other two are
// measured *out*: `-` and `!` mean something only inside a bracket
// expression, and the `[` that would open one is already escaped by the time
// either is reached, so the flag leaves them alone.
const patternQuoteMeta = patternMeta + extendedPatternMeta + ">]"

// quotePatternMeta writes v with every character in patternQuoteMeta marked
// by a backslash.
//
// Byte at a time on purpose: every character in the set is ASCII, and no byte
// of a multibyte sequence can collide with one, so there is nothing for a
// rune decode to protect and a value holding invalid UTF-8 passes through
// unharmed — measured, `${(b)v}` on the two bytes `0xff` `*` is `0xff \ *`.
func quotePatternMeta(v string) string {
	if !strings.ContainsAny(v, patternQuoteMeta) {
		return v
	}
	var b strings.Builder
	b.Grow(len(v) + 8)
	for i := 0; i < len(v); i++ {
		if strings.IndexByte(patternQuoteMeta, v[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	return b.String()
}

// patternQuoteFlagApplies reports whether this group carries `(b)`.
func patternQuoteFlagApplies(e *syntax.ParamExpr) bool {
	return e != nil && e.HasFlags && strings.ContainsRune(e.Flags, 'b')
}
