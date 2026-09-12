// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
)

// How `%q` writes a value so that reading it back gives the value again.
//
// That round trip is the whole contract, and it is what this file exists for:
// the previous answer escaped a newline with a backslash, which is a *line
// continuation* — read back by any shell in the panel, `a\<newline>b` is `ab`
// and not `a<newline>b`, so the output was not merely differently spelled
// from the panel's, it was wrong (#1707).
//
// Three styles and one absence. They were measured byte by byte rather than
// derived from a rule, because the rules turn out not to agree anywhere:
//
//	                       bash 5.3         zsh 5.9.2        ksh93u+
//	a<newline>b            $'a\nb'          a$'\n'b          $'a\nb'
//	a<esc>b                $'a\Eb'          a$'\033'b        $'a\Eb'
//	a<vt>b                 $'a\vb'          a$'\v'b          $'a\x0bb'
//	a<0x01>b               $'a\001b'        a$'\001'b        $'a\x01b'
//	a<0xff>b               $'a\377b'        a$'\377'b        $'a\xffb'
//	a b                    a\ b             a\ b             'a b'
//	a'b                    a\'b             a\'b             $'a\'b'
//	a\b                    a\\b             a\\b             'a\b'
//	a!b                    a\!b             a!b              a!b
//	a#b                    a#b              a\#b             'a#b'
//	=ab                    =ab              \=ab             '=ab'
//
// So: bash moves the **whole word** into `$'…'` as soon as one byte cannot be
// written as itself; zsh wraps each such byte in a `$'…'` of its own and
// leaves the rest backslash-escaped; ksh93 has three shapes — bare, `'…'`,
// and `$'…'` with hex escapes — and picks by what the value holds.
//
// Measured 2026-09-12, LC_ALL=C, with the value handed over as an argument
// rather than through a command substitution: a substitution eats a trailing
// newline and re-encodes a high byte, which is how an earlier reading of the
// same question came back with a byte boundary at 0xa0 that is not there.

// unwritableByte reports a byte no shell in the panel will write as itself
// inside a word: the C0 controls, DEL, and everything above it.
//
// Asked of single bytes only. A valid multibyte rune is written as itself by
// all three — measured, `aĉb` comes back `aĉb` in every column — and
// eachQuotableByte is what keeps the two apart.
func unwritableByte(c byte) bool { return c < 0x20 || c >= 0x7f }

// hasUnwritableByte is that question about a whole value, and it is the test
// two of the three styles switch on.
func hasUnwritableByte(s string) bool {
	found := false
	eachQuotableByte(s, func(_ int, c byte) {
		if unwritableByte(c) {
			found = true
		}
	}, func(int, string) {})
	return found
}

// bashQuotableByte is the set bash escapes with a backslash, expressed as the
// difference from the set this package already had for zsh — see
// quotableByte, whose table `${(q)…}`, `${(q-)…}` and `:q` share.
//
// One predicate against another rather than a second table, because the two
// sets agree on every byte but four and a second table is how they would come
// to disagree about a fifth. Measured over every printable byte at three
// positions, which is where the four came from:
//
//	`!` and `,`  bash escapes them, zsh never does
//	`#`          both escape a leading one; zsh escapes it anywhere
//	`=`          zsh escapes a leading one, bash never does
func bashQuotableByte(i int, c byte) bool {
	switch c {
	case '!', ',':
		return true
	case '#':
		return i == 0
	case '=':
		return false
	}
	return quotableByte(i, c)
}

// kshQuotableByte is the same difference for ksh93, whose `'…'` decision is
// taken over the whole value: one byte in this set puts the *value* in
// quotes rather than that byte behind a backslash.
//
//	`^`   zsh escapes it, ksh93 does not
//	`~`   ksh93 quotes it wherever it stands, not only at the start
//	`=`   both only at the start
func kshQuotableByte(i int, c byte) bool {
	switch c {
	case '^':
		return false
	case '~':
		return true
	}
	return quotableByte(i, c)
}

// ansiCWordQuote is bash's `%q`: backslash escaping while every byte can be
// written as itself, and the whole word inside one `$'…'` as soon as one
// cannot.
//
// Whole word and not per byte, which is visible on a value that holds both
// kinds of trouble: `a b<newline>c` is `$'a b\nc'`, with the space no longer
// escaped because the quotes already cover it.
func ansiCWordQuote(s string) string {
	if s == "" {
		return "''"
	}
	if !hasUnwritableByte(s) {
		var b strings.Builder
		eachQuotableByte(s, func(i int, c byte) {
			if bashQuotableByte(i, c) {
				b.WriteByte('\\')
			}
			b.WriteByte(c)
		}, func(_ int, raw string) { b.WriteString(raw) })
		return b.String()
	}
	var b strings.Builder
	b.WriteString("$'")
	eachQuotableByte(s, func(_ int, c byte) {
		switch {
		case c == '\'' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case unwritableByte(c):
			b.WriteString(bashControlEscape(c))
		default:
			b.WriteByte(c)
		}
	}, func(_ int, raw string) { b.WriteString(raw) })
	b.WriteByte('\'')
	return b.String()
}

// bashControlEscape is controlEscape with one difference, and it is the only
// one over the whole byte range: bash writes escape as `\E` where zsh writes
// `\033`. Everything else is the same seven names and the same three-digit
// octal.
func bashControlEscape(c byte) string {
	if c == 0x1b {
		return `\E`
	}
	return controlEscape(c)
}

// kshSingleQuote is ksh93's `%q`, which has three shapes rather than two.
//
// A value it can write bare it writes bare — `abc` is `abc`, where the other
// two agree. A value holding a byte it cannot write, or a `'`, goes into
// `$'…'`; anything else that needs quoting goes into plain `'…'`, where a
// backslash is *not* doubled because a single-quoted backslash is already
// itself. That last row is the one a `'\”`-style quoter gets wrong: `a\b` is
// `'a\b'` here and `'a\b'` read back is `a\b`.
func kshSingleQuote(s string) string {
	if s == "" {
		return "''"
	}
	if hasUnwritableByte(s) || strings.IndexByte(s, '\'') >= 0 {
		var b strings.Builder
		b.WriteString("$'")
		eachQuotableByte(s, func(_ int, c byte) {
			switch {
			case c == '\'' || c == '\\':
				b.WriteByte('\\')
				b.WriteByte(c)
			case unwritableByte(c):
				b.WriteString(kshControlEscape(c))
			default:
				b.WriteByte(c)
			}
		}, func(_ int, raw string) { b.WriteString(raw) })
		b.WriteByte('\'')
		return b.String()
	}
	quote := false
	eachQuotableByte(s, func(i int, c byte) {
		if kshQuotableByte(i, c) {
			quote = true
		}
	}, func(int, string) {})
	if quote {
		return "'" + s + "'"
	}
	return s
}

// kshControlEscape is the third spelling of one byte, and the reason it is a
// third rather than a second: ksh93 writes two-digit lower-case hex where the
// other two write octal, and it has no `\v` — a vertical tab is `\x0b` there
// and `\v` in both of the others. The named set it does have is the same
// seven minus that one, with `\E` for escape as bash has.
func kshControlEscape(c byte) string {
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\f':
		return `\f`
	case '\n':
		return `\n`
	case '\r':
		return `\r`
	case '\t':
		return `\t`
	case 0x1b:
		return `\E`
	}
	return fmt.Sprintf(`\x%02x`, c)
}
