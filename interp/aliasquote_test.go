// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// Four engines and no two alike. Every want here is what the shell that owns
// the style actually printed, quoted back exactly.
func TestTheFourAliasQuotingEngines(t *testing.T) {
	for _, c := range []struct {
		style       AliasQuotingStyle
		value, want string
		name        string
	}{
		// Always quotes; an embedded quote closes and reopens with a
		// backslashed one, and a value *ending* in a quote keeps the empty
		// pair that leaves behind.
		{AliasQuoteAlwaysEscaped, `ls`, `'ls'`, "bash, plain"},
		{AliasQuoteAlwaysEscaped, `echo x`, `'echo x'`, "bash, space"},
		{AliasQuoteAlwaysEscaped, `it's`, `'it'\''s'`, "bash, quote"},
		{AliasQuoteAlwaysEscaped, `echo 'hi'`, `'echo '\''hi'\'''`, "bash, trailing quote"},

		// Always quotes; an embedded quote is written as a double-quoted one,
		// and the empty tail is dropped.
		{AliasQuoteAlwaysDoubled, `ls`, `'ls'`, "dash, plain"},
		{AliasQuoteAlwaysDoubled, `it's`, `'it'"'"'s'`, "dash, quote"},
		{AliasQuoteAlwaysDoubled, `echo 'hi'`, `'echo '"'"'hi'"'"`, "dash, trailing quote"},

		// Quotes only when needed, and reaches for $'...' for a quote or a
		// control character.
		{AliasQuoteWhenNeededDollar, `ls`, `ls`, "ksh93, bare"},
		{AliasQuoteWhenNeededDollar, `echo x`, `'echo x'`, "ksh93, space"},
		{AliasQuoteWhenNeededDollar, `it's`, `$'it\'s'`, "ksh93, quote"},
		{AliasQuoteWhenNeededDollar, "a\tb", `$'a\tb'`, "ksh93, tab"},

		// Quotes only when needed, escapes a quote the way bash does, and
		// reaches for $'...' only for a control character.
		{AliasQuoteWhenNeededEscaped, `ls`, `ls`, "zsh, bare"},
		{AliasQuoteWhenNeededEscaped, `echo x`, `'echo x'`, "zsh, space"},
		{AliasQuoteWhenNeededEscaped, `it's`, `'it'\''s'`, "zsh, quote"},
		{AliasQuoteWhenNeededEscaped, `echo 'hi'`, `'echo '\''hi'\'`, "zsh, trailing quote"},
		{AliasQuoteWhenNeededEscaped, "a\tb", `$'a\tb'`, "zsh, tab"},

		// The characters that do not force quotes in the two that ask.
		{AliasQuoteWhenNeededDollar, `a-b.c/d:e@f`, `a-b.c/d:e@f`, "ksh93, punctuation that is safe"},
		{AliasQuoteWhenNeededEscaped, `a"b`, `'a"b'`, "zsh, a double quote is not safe"},
		{AliasQuoteWhenNeededEscaped, `a$b`, `'a$b'`, "zsh, a dollar is not safe"},
		// An empty value is never bare — `a=` would read back as a lookup.
		{AliasQuoteWhenNeededDollar, ``, `''`, "ksh93, empty"},
	} {
		got, _ := aliasRunWithValue(t, c.style, c.value)
		if got != "a="+c.want {
			t.Errorf("%s: value %q spelled %q, want %q", c.name, c.value, strings.TrimPrefix(got, "a="), c.want)
		}
	}
}

// With no dialect chosen the substrate refuses rather than picking a spelling.
func TestNoAliasQuotingStyleMeansNoAnswer(t *testing.T) {
	out, _ := aliasRun(t, func(s *Semantics) { s.AliasQuoting = AliasQuotingUnspecified },
		Diagnostics{}, `alias a=1; alias a`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want the unspecified refusal", out)
	}
}
