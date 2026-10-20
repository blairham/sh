// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// An `&&` or `||` with nothing after it names the token it found. This shell
// quotes it and gives the line, and answers the end of input the same way as
// any other token rather than as a construct left open.
//
// Measured 2026-09-07, ksh93u+ from a script file under `env -i`:
//
//	$ ksh -n s.sh          # echo a && fi
//	s.sh: syntax error at line 1: `fi' unexpected
//	$ ksh -n s.sh          # { : || \n }
//	s.sh: syntax error at line 2: `}' unexpected
//	$ ksh -n s.sh          # echo one &&\n
//	s.sh: syntax error at line 2: `end of file' unexpected
//
// All of these answered `expected a command after &&` before this — a sentence
// no shell in the panel writes. #1115 fixed the same mistake for the bar and
// did not reach the and-or.
//
// `echo one && ; echo two` is deliberately not here: this shell *accepts* that
// line, and the acceptance is #1142's, not this test's.
func TestAnAndOrWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one && | echo two\n", "syntax error at line 1: `|' unexpected"},
		{"echo one && && echo two\n", "syntax error at line 1: `&&' unexpected"},
		{"echo a && fi\n", "syntax error at line 1: `fi' unexpected"},
		{"echo a || done\n", "syntax error at line 1: `done' unexpected"},
		{"{ : ||\n}\n", "syntax error at line 2: `}' unexpected"},
		{"( : ||\n)\n", "syntax error at line 2: `)' unexpected"},
		{"while false; do : ||\ndone\n", "syntax error at line 2: `done' unexpected"},
		{"echo one &&\n", "syntax error at line 2: `end of file' unexpected"},
		{"echo one ||\n", "syntax error at line 2: `end of file' unexpected"},
	} {
		_, err := syntax.Parse(tc.src, ksh.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		if got := ksh.Diagnostics().ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
	}
}

// This shell has no open-ended and-or, which is the discriminating half of the
// measurement: it takes `echo one && ; echo two` and refuses `echo one &&` at
// the end of input and `{ : || ⏎ }`, so its leniency is about the `;` standing
// there (#1142) and not about the list being allowed to close.
func TestNoOpenEndedAndOr(t *testing.T) {
	if ksh.Dialect().OpenEndedAndOr {
		t.Error("ksh93 refuses an and-or that ends with its operator before a closer")
	}
}
