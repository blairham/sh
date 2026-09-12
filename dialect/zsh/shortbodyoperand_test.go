// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// An `&` where a loop's condition belongs is named where it stands.
//
// Measured on zsh 5.9.2, 2026-09-12, `env -i PATH=/usr/bin:/bin` with a
// scratch HOME and ZDOTDIR, `-n` over a script file. This shell takes an
// empty condition and a short body, so the `&` had been stepped past and the
// `do` two tokens later was the first thing refused — which named a token the
// script's trouble is not at.
//
// The `&` is the odd one out in this shell and the rows below are the ones
// that hold: `|`, `&&`, `||` and `|&` in the same position are *accepted*
// there — `zsh -n -c 'if | :'` parses — so their wording is downstream of a
// grammar difference and is not this (#2235).
func TestALeadingAmpersandInALoopHeaderIsNamedWhereItStands(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"while & do :; done\n", "parse error near `&'"},
		{"until & do :; done\n", "parse error near `&'"},
		// The `if` header already said this, and is here so a change that
		// moved the two apart again would show.
		{"if & then :; fi\n", "parse error near `&'"},
	} {
		_, err := syntax.Parse(tc.src, zsh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := zsh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
