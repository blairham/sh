// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "strings"

// How `alias` spells the value it lists back.
//
// Four engines, and no two agree. The listing is meant to be text the shell
// could read again, so each one quotes whatever its own parser would need —
// which is why this is a policy rather than one function with flags.
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

// AliasQuotingStyle is how a dialect spells an alias value in a listing.
type AliasQuotingStyle int

const (
	// AliasQuotingUnspecified is no answer, and is refused like any other.
	AliasQuotingUnspecified AliasQuotingStyle = iota
	// AliasQuoteAlwaysEscaped always single-quotes and writes an embedded
	// quote as `'\''`: bash.
	AliasQuoteAlwaysEscaped
	// AliasQuoteAlwaysDoubled always single-quotes and writes an embedded
	// quote as `'"'"'`: dash.
	AliasQuoteAlwaysDoubled
	// AliasQuoteWhenNeededDollar leaves a plain value bare and reaches for
	// `$'...'` where a quote or a control character appears: ksh93.
	AliasQuoteWhenNeededDollar
	// AliasQuoteWhenNeededEscaped leaves a plain value bare, writes an
	// embedded quote as `'\''`, and reaches for `$'...'` only for a control
	// character: zsh.
	AliasQuoteWhenNeededEscaped
)

func (a AliasQuotingStyle) String() string {
	switch a {
	case AliasQuoteAlwaysEscaped:
		return "AliasQuoteAlwaysEscaped"
	case AliasQuoteAlwaysDoubled:
		return "AliasQuoteAlwaysDoubled"
	case AliasQuoteWhenNeededDollar:
		return "AliasQuoteWhenNeededDollar"
	case AliasQuoteWhenNeededEscaped:
		return "AliasQuoteWhenNeededEscaped"
	}
	return "AliasQuotingUnspecified"
}

// quoteAliasValue spells a value the way this dialect's `alias` lists it.
func (r *Runner) quoteAliasValue(v string) string {
	switch r.sem().AliasQuoting {
	case AliasQuoteAlwaysEscaped:
		return singleQuoted(v, `'\''`, false)
	case AliasQuoteAlwaysDoubled:
		return singleQuoted(v, `'"'"'`, true)
	case AliasQuoteWhenNeededDollar:
		switch {
		case hasControl(v):
			return dollarQuoted(v)
		case strings.ContainsRune(v, '\''):
			return dollarQuoted(v)
		case aliasValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	case AliasQuoteWhenNeededEscaped:
		switch {
		case hasControl(v):
			return dollarQuoted(v)
		case aliasValueIsBare(v):
			return v
		}
		return singleQuoted(v, `'\''`, true)
	}
	r.diagf("how `alias` spells a value: the shells disagree here and no dialect was chosen\n")
	r.status = 2
	r.unspecified = true
	return v
}

// aliasValueIsBare reports whether a value can be listed with no quotes at
// all, which the two that ask only do for a value made of ordinary characters.
func aliasValueIsBare(v string) bool {
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
