// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"strings"
)

// How a shell spells a value it lists back — an alias's replacement, a
// trap's action.
//
// Four engines, and no two agree. The listing is meant to be text the shell
// could read again, so each one quotes whatever its own parser would need —
// which is why this is a policy rather than one function with flags.
//
// The style is shared; which style a builtin uses is not. zsh spells an alias
// holding a tab as `$'a\tb'` and a trap holding one as `'a<tab>b'`, so the
// two are separate fields over the same vocabulary.
//
//	value        bash              dash                ksh93          zsh
//	ls           'ls'              'ls'                ls             ls
//	echo x       'echo x'          'echo x'            'echo x'       'echo x'
//	it's         'it'\''s'         'it'"'"'s'          $'it\'s'       'it'\''s'
//	a<tab>b      'a<tab>b'         'a<tab>b'           $'a\tb'        $'a\tb'
//
// Two of them quote always and two only when the value needs it; two escape an
// embedded quote by closing and reopening with a backslash and one by closing
// and reopening with a double-quoted quote; and two reach for `$'...'` when the
// value holds a control character.

// ListingQuotingStyle is how a dialect spells a value it lists back.
type ListingQuotingStyle int

const (
	// ListingQuotingUnspecified is no answer, and is refused like any other.
	ListingQuotingUnspecified ListingQuotingStyle = iota
	// ListingQuoteAlwaysEscaped always single-quotes and writes an embedded
	// quote as `'\''`: bash.
	ListingQuoteAlwaysEscaped
	// ListingQuoteAlwaysDoubled always single-quotes and writes an embedded
	// quote as `'"'"'`: dash.
	ListingQuoteAlwaysDoubled
	// ListingQuoteWhenNeededDollar leaves a plain value bare and reaches for
	// `$'...'` where a quote or a control character appears: ksh93.
	ListingQuoteWhenNeededDollar
	// ListingQuoteWhenNeededEscaped leaves a plain value bare, writes an
	// embedded quote as `'\''`, and reaches for `$'...'` only for a control
	// character: zsh's `alias`.
	ListingQuoteWhenNeededEscaped
	// ListingQuoteAlwaysDouble always double-quotes, escaping an embedded
	// backslash, backquote, dollar or double quote, and replaces the double
	// quotes with `$'...'` when the value holds a control character: bash's
	// `declare -p`, which is not the single-quoting style its `alias` and
	// `trap` use.
	ListingQuoteAlwaysDouble
	// ListingQuoteWhenNeededPlain leaves a plain value bare and single-quotes
	// everything else, reaching for `$'...'` never: zsh's `trap`.
	//
	// zsh arrives at that output a different way — it parses the action when
	// the trap is set, refuses one it cannot parse, and lists the parse back
	// rather than the text it was given, so `a<tab>b` lists as `a b`. For an
	// action that is an ordinary command the two agree, which is every
	// action in the corpus. Where they do not, this is the closer of the two
	// answers available, and re-printing the parse is its own question.
	ListingQuoteWhenNeededPlain
)

func (a ListingQuotingStyle) String() string {
	switch a {
	case ListingQuoteAlwaysEscaped:
		return "ListingQuoteAlwaysEscaped"
	case ListingQuoteAlwaysDoubled:
		return "ListingQuoteAlwaysDoubled"
	case ListingQuoteWhenNeededDollar:
		return "ListingQuoteWhenNeededDollar"
	case ListingQuoteWhenNeededEscaped:
		return "ListingQuoteWhenNeededEscaped"
	case ListingQuoteAlwaysDouble:
		return "ListingQuoteAlwaysDouble"
	case ListingQuoteWhenNeededPlain:
		return "ListingQuoteWhenNeededPlain"
	}
	return "ListingQuotingUnspecified"
}

// quoteListedValue spells a value the way this dialect lists it back, in the
// style the calling builtin uses. What is being listed is named so the
// refusal can say which question went unanswered.
func (r *Runner) quoteListedValue(style ListingQuotingStyle, what, v string) string {
	switch style {
	case ListingQuoteAlwaysEscaped:
		return singleQuoted(v, `'\''`, false)
	case ListingQuoteAlwaysDoubled:
		return singleQuoted(v, `'"'"'`, true)
	case ListingQuoteWhenNeededDollar:
		switch {
		case hasControl(v):
			return r.dollarQuoted(v)
		case strings.ContainsRune(v, '\''):
			return r.dollarQuoted(v)
		case listedValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteWhenNeededEscaped:
		switch {
		case hasControl(v):
			return r.dollarQuoted(v)
		case listedValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteWhenNeededPlain:
		if listedValueIsBare(v) {
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteAlwaysDouble:
		if hasControl(v) {
			return r.dollarQuoted(v)
		}
		return doubleQuoted(v)
	}
	r.diagf("%s\n", r.unanswered("how "+what+" spells a value"))
	r.status = 2
	r.unspecified = true
	return v
}

// listedValueIsBare reports whether a value can be listed with no quotes at
// all, which the styles that ask only do for a value made of ordinary
// characters.
func listedValueIsBare(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if isLetter(c) || isDigit(c) || strings.IndexByte("_-./:@+,%^", c) >= 0 {
			continue
		}
		return false
	}
	return true
}

func hasControl(v string) bool {
	for i := 0; i < len(v); i++ {
		if v[i] < 0x20 || v[i] == 0x7f {
			return true
		}
	}
	return false
}

// singleQuoted wraps in single quotes, replacing each embedded quote with the
// dialect's way of writing one.
//
// Every escape here closes the quoted run, writes a quote somehow, and reopens
// it. When the value *ends* in a quote that reopened run is empty, and the
// panel splits on whether to leave it there: bash keeps the empty pair, and
// dash and zsh drop the reopen along with its closing quote.
//
// Only a value ending in a quote can show it, which is why it took a probe
// written to end in one.
func singleQuoted(v, escape string, trimEmptyTail bool) string {
	quoted := "'" + strings.ReplaceAll(v, "'", escape) + "'"
	if trimEmptyTail && strings.HasSuffix(v, "'") {
		// Drop the closing quote and the reopening one the last escape
		// wrote, which between them wrap nothing.
		return quoted[:len(quoted)-2]
	}
	return quoted
}

// doubleQuoted wraps in double quotes, escaping the four characters that are
// live inside them: backslash, backquote, dollar and the quote itself.
func doubleQuoted(v string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(v); i++ {
		if c := v[i]; c == '\\' || c == '`' || c == '$' || c == '"' {
			b.WriteByte('\\')
		}
		b.WriteByte(v[i])
	}
	b.WriteByte('"')
	return b.String()
}

// dollarQuoted writes `$'...'`, the spelling that can carry a control
// character.
func (r *Runner) dollarQuoted(v string) string {
	style := r.sem().ListingControlEscape
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(v); i++ {
		switch c := v[i]; {
		case c == '\'':
			b.WriteString(`\'`)
		case c == '\\':
			b.WriteString(`\\`)
		case c >= 0x20 && c != 0x7f:
			b.WriteByte(c)
		default:
			b.WriteString(controlEscaped(style, c))
		}
	}
	b.WriteString("'")
	return b.String()
}

// ControlEscapeStyle is how a `$'...'` listing spells a byte the quotes cannot
// carry as itself. Every shell that lists with `$'...'` escapes *every*
// control byte — a listing is meant to be readable back, and a raw NUL makes
// the whole output binary, which is what `grep` says of it — and no two of
// them spell one the same way. Measured 2026-09-05 from
// `v=$'\a\b\t\n\v\f\r\e\001\037\177'` in each:
//
//	bash    \a \b \t \n \v \f \r \E then three octal digits: \001 \037 \177
//	ksh93   \a \b \t \n     \f \r \E then two hex digits:     \x0b \x01 \x7f
//	zsh             \t \n                 then a caret pair:      \C-G \C-A \C-?
//
// The named sets are not the same either — ksh93 has no `\v` and zsh has only
// the two — so the style names one whole vocabulary rather than a fallback.
// dash never reaches here: it single-quotes every value it lists and writes a
// control byte as itself.
type ControlEscapeStyle int

const (
	// ControlEscapeUnspecified is no answer. It is not refused, because the
	// question is only reachable from a quoting style that has already been
	// answered; it spells the numeric fallback the way POSIX's `printf` does.
	ControlEscapeUnspecified ControlEscapeStyle = iota
	// ControlEscapeOctal is bash's: the widest named set, then `\NNN`.
	ControlEscapeOctal
	// ControlEscapeHex is ksh93's: the same named set without `\v`, then
	// `\xNN` in lowercase.
	ControlEscapeHex
	// ControlEscapeCaret is zsh's: `\t` and `\n` alone, then `\C-X` — the
	// byte with bit 6 flipped, so NUL is `\C-@`, 1 is `\C-A` and delete is
	// `\C-?`.
	ControlEscapeCaret
)

func (c ControlEscapeStyle) String() string {
	switch c {
	case ControlEscapeOctal:
		return "ControlEscapeOctal"
	case ControlEscapeHex:
		return "ControlEscapeHex"
	case ControlEscapeCaret:
		return "ControlEscapeCaret"
	}
	return "ControlEscapeUnspecified"
}

// controlEscaped spells one control byte in the given style.
func controlEscaped(style ControlEscapeStyle, c byte) string {
	if style == ControlEscapeCaret {
		switch c {
		case '\t':
			return `\t`
		case '\n':
			return `\n`
		}
		return fmt.Sprintf(`\C-%c`, c^0x40)
	}
	switch c {
	case '\a':
		return `\a`
	case '\b':
		return `\b`
	case '\t':
		return `\t`
	case '\n':
		return `\n`
	case '\v':
		if style == ControlEscapeHex {
			// The one byte ksh93 leaves out of the named set.
			break
		}
		return `\v`
	case '\f':
		return `\f`
	case '\r':
		return `\r`
	case 0x1b:
		return `\E`
	}
	if style == ControlEscapeHex {
		return fmt.Sprintf(`\x%02x`, c)
	}
	return fmt.Sprintf(`\%03o`, c)
}
