// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

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
			return dollarQuoted(v)
		case strings.ContainsRune(v, '\''):
			return dollarQuoted(v)
		case listedValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteWhenNeededEscaped:
		switch {
		case hasControl(v):
			return dollarQuoted(v)
		case listedValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case ListingQuoteWhenNeededPlain:
		if listedValueIsBare(v) {
			return v
		}
		return singleQuoted(v, `'\''`, true)
	}
	r.diagf("how %s spells a value: the shells disagree here and no dialect was chosen\n", what)
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

// dollarQuoted writes `$'...'`, the spelling that can carry a control
// character.
func dollarQuoted(v string) string {
	var b strings.Builder
	b.WriteString("$'")
	for i := 0; i < len(v); i++ {
		switch c := v[i]; c {
		case '\'':
			b.WriteString(`\'`)
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteString("'")
	return b.String()
}
