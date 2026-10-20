// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/syntax"
)

// An `&&` or `||` with nothing after it names the token it found. The same
// rule a bar follows since #1115, which fixed it for the bar and left the
// and-or answering `expected a command after &&` in every dialect.
//
// Measured 2026-09-07, dash from a script file under `env -i`:
//
//	$ dash -n s.sh          # echo one && ; echo two
//	s.sh: 1: Syntax error: ";" unexpected
//	$ dash -n s.sh          # echo a && fi
//	s.sh: 1: Syntax error: "fi" unexpected
//	$ dash -n s.sh          # echo one &&\n
//	s.sh: 2: Syntax error: end of file unexpected
//
// A command may begin after `&&`, so a reserved word standing there is
// reserved — this is the shell that names a word's *class* instead of quoting
// it, and it quotes this one. The operand form would say `word unexpected`
// here and nowhere else, which is exactly the mistake #1115 found for the bar.
// The whole rendered line, location included, because the *position* is as
// easy to get wrong as the sentence: a blame that stayed on the operator's
// line would report `1` for `{ : || \u23ce }` where this shell says `2`.
func TestAnAndOrWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one && ; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"echo one || ; echo two\n", "s.sh: 1: Syntax error: \";\" unexpected\n"},
		{"echo one && & echo two\n", "s.sh: 1: Syntax error: \"&\" unexpected\n"},
		{"echo one && | echo two\n", "s.sh: 1: Syntax error: \"|\" unexpected\n"},
		{"echo one && && echo two\n", "s.sh: 1: Syntax error: \"&&\" unexpected\n"},
		{"echo a && fi\n", "s.sh: 1: Syntax error: \"fi\" unexpected\n"},
		{"echo a || done\n", "s.sh: 1: Syntax error: \"done\" unexpected\n"},
		{"{ : ||\n}\n", "s.sh: 2: Syntax error: \"}\" unexpected\n"},
		{"( : ||\n)\n", "s.sh: 2: Syntax error: \")\" unexpected\n"},
		{"echo one &&\n", "s.sh: 2: Syntax error: end of file unexpected\n"},
		{"echo one ||\n", "s.sh: 2: Syntax error: end of file unexpected\n"},
	} {
		_, err := syntax.Parse(tc.src, dash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := dash.Diagnostics().ForScript()
		if got := d.ParseDiagnostic("s.sh", "", err, tc.src); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// This shell has no open-ended and-or, which is the other half: the flag is
// zsh's alone and an edit to the preset cannot turn it on here silently.
func TestNoOpenEndedAndOr(t *testing.T) {
	if dash.Dialect().OpenEndedAndOr {
		t.Error("dash refuses an and-or that ends with its operator")
	}
}
