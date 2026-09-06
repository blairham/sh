// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// The `Q` expansion flag: one level of quoting removed from the value, and
// nothing else done to it.
//
// **Removed, not expanded**, and that is the whole of the flag. Measured on
// zsh 5.9.2:
//
//	v='"$x"'       ${(Q)v}   $x          with x set, so the quotes went and the name did not
//	v='$(echo hi)' ${(Q)v}   $(echo hi)  a substitution stays ten characters of text
//	v="'a b' c"    ${(Q)v}   a b c       one word, because it does not split either
//	v='a\ b'       ${(Q)v}   a b
//	v="\$'a\tb'"   ${(Q)v}   a<TAB>b     `$'…'` is decoded
//	v='$"x"'       ${(Q)v}   $x          `$"` is not a quote, so the `$` is text
//	v="'unterm"    ${(Q)v}   'unterm     an opener with no closer is left alone
//
// So this is the lexer's quote removal with every expansion switched off —
// which is why it is written here rather than reached through the parser: a
// parse of `"$x"` finds a parameter, and the answer is that the parameter is
// not looked at.

// unquoteFlagged removes one level of quoting from v.
//
// `'…'` takes its contents whole, `"…"` gives a backslash meaning before
// `"`, “ ` “, `$` and `\` and drops a `\`-newline pair altogether, leaving
// the backslash in place elsewhere — `"a\qb"` is `a\qb` — `$'…'` is decoded
// by the same reader the construct itself uses, and a bare backslash escapes
// whatever follows it. A trailing backslash is dropped.
//
// An unterminated quote is **not** an error and not a truncation: the opener
// and everything after it come back as they were written, which is measured
// and is what a value assembled out of a shell line needs.
func (r *Runner) unquoteFlagged(v string) string {
	var b strings.Builder
	for i := 0; i < len(v); {
		switch c := v[i]; {
		case c == '\\':
			if i+1 >= len(v) {
				return b.String()
			}
			if v[i+1] != '\n' {
				// A `\`-newline pair is a line continuation and leaves
				// nothing behind: measured, `"a\<newline>b"` is `ab`.
				b.WriteByte(v[i+1])
			}
			i += 2
		case c == '\'':
			end := strings.IndexByte(v[i+1:], '\'')
			if end < 0 {
				b.WriteString(v[i:])
				return b.String()
			}
			b.WriteString(v[i+1 : i+1+end])
			i += end + 2
		case c == '"':
			end := indexDoubleQuoteEnd(v[i+1:])
			if end < 0 {
				b.WriteString(v[i:])
				return b.String()
			}
			writeDoubleQuoted(&b, v[i+1:i+1+end])
			i += end + 2
		case c == '$' && i+1 < len(v) && v[i+1] == '\'':
			end := indexDollarSingleEnd(v[i+2:])
			if end < 0 {
				b.WriteString(v[i:])
				return b.String()
			}
			b.WriteString(r.expandDollarSingle(v[i+2 : i+2+end]))
			i += end + 3
		default:
			n := verbatimGroup(v[i:])
			switch {
			case n > 0:
				b.WriteString(v[i : i+n])
				i += n
			case n < 0:
				b.WriteString(v[i:])
				return b.String()
			default:
				b.WriteByte(c)
				i++
			}
		}
	}
	return b.String()
}

// verbatimGroup measures a substitution written at the start of s: a
// backquoted run, `$(…)` or `${…}`. It returns the group's length, 0 when s
// does not begin one, and -1 when it begins one that is never closed.
//
// These come back **as written**, opener and all, which is the same fact as
// `Q` performing no expansion — and the reason they are a region rather than
// ordinary text is measured: `${(Q)v}` on `"a$(b"c")d"` is `a$(b"c")d`, so
// the quotes inside the substitution neither closed the outer one nor were
// removed themselves, and `"a`b"c`d"` is `a`b"c`d` for the same reason. An
// unclosed one takes the enclosing construct with it — `"a$(x b"` comes back
// whole — which is why the third answer exists rather than a bool.
func verbatimGroup(s string) int {
	switch {
	case s[0] == '`':
		if end := indexBackquoteEnd(s[1:]); end >= 0 {
			return end + 2
		}
		return -1
	case strings.HasPrefix(s, "$("):
		return balancedGroup(s, '(', ')')
	case strings.HasPrefix(s, "${"):
		return balancedGroup(s, '{', '}')
	}
	return 0
}

// balancedGroup measures `$` plus a bracketed run, counting nested pairs so
// that `$(b(c))` is one group. -1 when the run never closes.
func balancedGroup(s string, opener, closer byte) int {
	depth := 0
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case opener:
			depth++
		case closer:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

// indexBackquoteEnd finds the “ ` “ that closes a backquoted run, skipping
// the ones a backslash claims.
func indexBackquoteEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '`':
			return i
		}
	}
	return -1
}

// indexDoubleQuoteEnd finds the `"` that closes a double-quoted run,
// skipping the ones a backslash claims and the ones inside a substitution,
// and returns -1 when there is none.
//
// Separate from the writing below on purpose: an unterminated run comes back
// **as written**, opener and all, so nothing may be emitted until the closer
// is known to exist. Writing as it scanned is what made `"unterm` answer
// `unterm"unterm` — the partial contents, and then the raw text after them.
func indexDoubleQuoteEnd(s string) int {
	for i := 0; i < len(s); {
		switch s[i] {
		case '\\':
			i += 2
		case '"':
			return i
		default:
			n := verbatimGroup(s[i:])
			if n < 0 {
				return -1
			}
			i += max(n, 1)
		}
	}
	return -1
}

// writeDoubleQuoted writes the contents of a double-quoted run: the text with
// a backslash removed only where it claims one of the characters it reaches
// — measured, `"a\qb"` keeps both bytes — and with a substitution copied
// through untouched.
func writeDoubleQuoted(b *strings.Builder, s string) {
	for i := 0; i < len(s); {
		switch {
		case s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\"`$\\\n", s[i+1]) >= 0:
			if s[i+1] != '\n' {
				b.WriteByte(s[i+1])
			}
			i += 2
		default:
			n := verbatimGroup(s[i:])
			if n > 0 {
				b.WriteString(s[i : i+n])
				i += n
				continue
			}
			b.WriteByte(s[i])
			i++
		}
	}
}

// indexDollarSingleEnd finds the `'` that closes a `$'…'`, skipping the ones
// a backslash claims, and returns -1 when there is none.
func indexDollarSingleEnd(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '\'':
			return i
		}
	}
	return -1
}
