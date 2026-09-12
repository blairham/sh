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
		style       ListingQuotingStyle
		value, want string
		name        string
	}{
		// Always quotes; an embedded quote closes and reopens with a
		// backslashed one, and a value *ending* in a quote keeps the empty
		// pair that leaves behind.
		{ListingQuoteAlwaysEscaped, `ls`, `'ls'`, "always-escaped, plain"},
		{ListingQuoteAlwaysEscaped, `echo x`, `'echo x'`, "always-escaped, space"},
		{ListingQuoteAlwaysEscaped, `it's`, `'it'\''s'`, "always-escaped, quote"},
		{ListingQuoteAlwaysEscaped, `echo 'hi'`, `'echo '\''hi'\'''`, "always-escaped, trailing quote"},

		// Always quotes; an embedded quote is written as a double-quoted one,
		// and the empty tail is dropped.
		{ListingQuoteAlwaysDoubled, `ls`, `'ls'`, "always-doubled, plain"},
		{ListingQuoteAlwaysDoubled, `it's`, `'it'"'"'s'`, "always-doubled, quote"},
		{ListingQuoteAlwaysDoubled, `echo 'hi'`, `'echo '"'"'hi'"'"`, "always-doubled, trailing quote"},

		// Quotes only when needed, and reaches for $'...' for a quote or a
		// control character.
		{ListingQuoteWhenNeededDollar, `ls`, `ls`, "when-needed-dollar, bare"},
		{ListingQuoteWhenNeededDollar, `echo x`, `'echo x'`, "when-needed-dollar, space"},
		{ListingQuoteWhenNeededDollar, `it's`, `$'it\'s'`, "when-needed-dollar, quote"},
		{ListingQuoteWhenNeededDollar, "a\tb", `$'a\tb'`, "when-needed-dollar, tab"},

		// Quotes only when needed, escapes a quote the way ListingQuoteAlwaysEscaped does, and
		// reaches for $'...' only for a control character. The whole value goes
		// inside one pair of quotes, so a value ending in one keeps the empty
		// pair that its escape leaves behind — measured on bash 5.3.15 with a
		// bare `set`, which is where the style is used.
		{ListingQuoteWhenNeededEscaped, `ls`, `ls`, "when-needed-escaped, bare"},
		{ListingQuoteWhenNeededEscaped, `echo x`, `'echo x'`, "when-needed-escaped, space"},
		{ListingQuoteWhenNeededEscaped, `it's`, `'it'\''s'`, "when-needed-escaped, quote"},
		{ListingQuoteWhenNeededEscaped, `echo 'hi'`, `'echo '\''hi'\'''`, "when-needed-escaped, trailing quote"},
		// And the value that is one quote and nothing else, which that
		// build writes with no quotes around it at all.
		{ListingQuoteWhenNeededEscaped, `'`, `\'`, "when-needed-escaped, a lone quote"},
		{ListingQuoteAlwaysEscaped, `'`, `\'`, "always-escaped, a lone quote"},
		{ListingQuoteWhenNeededEscaped, "a\tb", `$'a\tb'`, "when-needed-escaped, tab"},

		// The same decision spelled in runs: a non-empty piece either side of
		// each quote gets its own pair, so no empty pair is ever written.
		// Measured on zsh 5.9.2, over `set`, `alias` and `typeset -p`.
		{ListingQuoteWhenNeededRuns, `ls`, `ls`, "when-needed-runs, bare"},
		{ListingQuoteWhenNeededRuns, `echo x`, `'echo x'`, "when-needed-runs, space"},
		{ListingQuoteWhenNeededRuns, `it's`, `'it'\''s'`, "when-needed-runs, quote"},
		{ListingQuoteWhenNeededRuns, `echo 'hi'`, `'echo '\''hi'\'`, "when-needed-runs, trailing quote"},
		{ListingQuoteWhenNeededRuns, `'x`, `\''x'`, "when-needed-runs, leading quote"},
		{ListingQuoteWhenNeededRuns, `'`, `\'`, "when-needed-runs, a lone quote"},
		{ListingQuoteWhenNeededRuns, `''`, `\'\'`, "when-needed-runs, two quotes"},
		{ListingQuoteWhenNeededRuns, "a\tb", `$'a\tb'`, "when-needed-runs, tab"},

		// The characters that do not force quotes in the two that ask.
		{ListingQuoteWhenNeededDollar, `a-b.c/d:e@f`, `a-b.c/d:e@f`, "when-needed-dollar, punctuation that is safe"},
		{ListingQuoteWhenNeededEscaped, `a"b`, `'a"b'`, "when-needed-escaped, a double quote is not safe"},
		{ListingQuoteWhenNeededEscaped, `a$b`, `'a$b'`, "when-needed-escaped, a dollar is not safe"},
		// An empty value is never bare — `a=` would read back as a lookup.
		{ListingQuoteWhenNeededDollar, ``, `''`, "when-needed-dollar, empty"},
	} {
		got, _ := aliasRunWithValue(t, c.style, c.value)
		if got != "a="+c.want {
			t.Errorf("%s: value %q spelled %q, want %q", c.name, c.value, strings.TrimPrefix(got, "a="), c.want)
		}
	}
}

// With no dialect chosen the substrate refuses rather than picking a spelling.
func TestNoListingQuotingStyleMeansNoAnswer(t *testing.T) {
	out, _ := aliasRun(t, func(s *Semantics) { s.AliasQuoting = ListingQuotingUnspecified },
		Diagnostics{}, `alias a=1; alias a`)
	if !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("said %q, want the unspecified refusal", out)
	}
}
