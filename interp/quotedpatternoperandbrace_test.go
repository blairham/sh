// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// balancedOperand runs one snippet with the quoted-pattern-operand nesting
// on or off, and nothing else moved.
//
// The other brace flag is set beside it on purpose in the `on` column: the
// shell that has one has the other, and the pair is what a caller gets. The
// row that turns only the new one off is what says they are separate.
func balancedOperand(t *testing.T, src string, on bool) string {
	t.Helper()
	out, _ := runGrammar(t, src+"\n", func(d *syntax.Dialect) {
		d.ParamSubstitution = true
		d.BareBraceNestsInExpansion = true
		d.BareBraceNestsInAQuotedPatternOperand = on
	}, nil)
	return strings.TrimSpace(out)
}

// A `{ … }` written inside a **double-quoted** parameter expansion's word is
// balanced in one column, so the expansion ends at the `}` that closes the
// expansion rather than at the one that closes the brace.
//
// Measured 2026-09-27 from `-c` under `env -i PATH=/usr/bin:/bin` on
// `/bin/ksh` `Version AJM 93u+ 2012-08-01`, `/opt/homebrew/bin/bash` 5.3.20,
// `/bin/bash` 3.2.57, `/opt/homebrew/bin/zsh` 5.9.2 and `/bin/dash` —
// `go version -m` says *not a Go executable* for each.
//
// **The `[}z}]` shape is the tell**: the expansion stopped at the brace's
// `}`, the `z` behind it came out as text, and the word's own `}` was then
// written as another character.
func TestABraceInAQuotedPatternOperandIsBalanced(t *testing.T) {
	for _, tc := range []struct{ name, src, on, off string }{
		{"a prefix trim", `s=x{y}z; echo "[${s#x{y}}]"`, `[z]`, `[}z}]`},
		{"the longest one", `s=x{y}z; echo "[${s##x{y}}]"`, `[z]`, `[}z}]`},
		{"a suffix trim", `s=a{b}c; echo "[${s%{b}c}]"`, `[a]`, `[a{b}cc}]`},
		{"the longest of those", `s=a{b}c; echo "[${s%%{b}c}]"`, `[a]`, `[a{b}cc}]`},
		{"a replacement", `s=x{y}z; echo "[${s/{y}/-}]"`, `[x-z]`, `[x}z/-}]`},

		// **A word operand stops at the first `}` in every column**,
		// including the one that balances above, which is what makes this
		// the operand kind rather than the shell. Both columns here are the
		// same text because the flag is not consulted for them at all.
		{"a default", `u=; echo "[${u:-x{y}z}]"`, `[x{yz}]`, `[x{yz}]`},
		{"an unset default", `u=; echo "[${u-x{y}z}]"`, `[z}]`, `[z}]`},
		{"an alternative", `u=; echo "[${u:+x{y}z}]"`, `[z}]`, `[z}]`},

		// A bracket expression in the same position is balanced in every
		// column already, which is what says a pattern operand is not
		// simply left unscanned.
		{"a bracket is not this", `s=aXb; echo "[${s#a[X]}]"`, `[b]`, `[b]`},

		// And it is the *scan* rather than what a group would have
		// produced: brace expansion reaches inside `${…}` in no column, so
		// `{a,q}` here is literal pattern text that matches nothing, and the
		// balancing column still balances it.
		{"and not brace expansion", `s=aXb; echo "[${s#{a,q}X}]"`, `[aXb]`, `[aXbX}]`},

		// Unquoted is the other flag and is not moved by this one: both
		// columns balance, because BareBraceNestsInExpansion is on in each.
		{"unquoted is the other flag", `s=x{y}z; echo "[${s#x{y}}]" >/dev/null; v=${s#x{y}}; echo "[$v]"`, `[z]`, `[z]`},
	} {
		if got := balancedOperand(t, tc.src, true); got != tc.on {
			t.Errorf("%s with the flag: %s = %q, want %q", tc.name, tc.src, got, tc.on)
		}
		if got := balancedOperand(t, tc.src, false); got != tc.off {
			t.Errorf("%s without it: %s = %q, want %q", tc.name, tc.src, got, tc.off)
		}
	}
}
