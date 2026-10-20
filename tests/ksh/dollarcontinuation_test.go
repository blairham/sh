// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/syntax"
)

// A line continuation between a `$` and what it introduces stops the `$` at a
// bare parameter and at a parenthesis here, and at every form inside double
// quotes. Measured 2026-09-16 on ksh93u+ 2012 from script files under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`, stdin on /dev/null, with `x=5` and
// `set -- a b`:
//
//	echo "[$\⏎x]"           [$x]        text, where bash, dash and zsh expand
//	echo "[$\⏎{x}]"         [${x}]      text, where zsh answers [{x}]
//	echo "[$\⏎1]"           [$1]
//	echo "[$\⏎(echo hi)]"   [$(echo hi)]
//	echo $\⏎(echo hi)       `(' unexpected, status 3 — the `$` stayed as text
//	                        and a `(` cannot begin a word
//	echo "[$\⏎{x}]" is the stop; outside quotes the brace is *not* one:
//	printf "[%s]\n" $\⏎{x}  [5]
//	printf "[%s]\n" $\⏎'a\tb'  a<tab>b — `$'…'` is not stopped at either
//
// The refusal row is the one worth keeping: a stop that left the `$` as text
// and then read the parenthesis as a construct anyway would print `hi` and
// pass every other line here.
func TestAContinuationStopsADollarAtABareParameterAndAParenthesis(t *testing.T) {
	d := ksh.Dialect()
	if got, want := d.ContinuationStopsADollarAt,
		syntax.DollarBareParameter|syntax.DollarParens; got != want {
		t.Errorf("ContinuationStopsADollarAt = %b, want %b", got, want)
	}
	if got := d.ContinuationStopsADollarAtInDoubleQuotes; got != syntax.EveryDollarForm {
		t.Errorf("ContinuationStopsADollarAtInDoubleQuotes = %b, want every form", got)
	}
	if d.DollarGoesWhenAContinuationStopsItAtABrace {
		t.Error("ksh93 leaves the `$` as text where it stops at a brace")
	}

	if _, err := syntax.Parse("echo $\\\n(echo hi)", d); err == nil {
		t.Error("`echo $\\⏎(echo hi)` parsed, want the `(` refused")
	}

	for _, tc := range []struct{ src, want string }{
		{"x=5; echo \"[$\\\nx]\"", "[$x]\n"},
		{"x=5; echo \"[$\\\n{x}]\"", "[${x}]\n"},
		{"set -- a b; echo \"[$\\\n1]\"", "[$1]\n"},
		{"echo \"[$\\\n(echo hi)]\"", "[$(echo hi)]\n"},
		// The two it does not stop at outside quotes.
		{"x=5; printf '[%s]\\n' $\\\n{x}", "[5]\n"},
		{"printf '[%s]\\n' $\\\n'a\\tb'", "[a\tb]\n"},
		// And an unquoted here-document body stops nothing at all, as the
		// refusal inside `${ }` does not reach one either.
		{"x=5; read -r l <<E\n[$\\\nx][$\\\n{x}]\nE\necho \"$l\"", "[5][5]\n"},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		if err != nil || out != tc.want {
			t.Errorf("%q: out = %q, err = %v, want %q", tc.src, out, err, strings.TrimSpace(tc.want))
		}
	}
}
