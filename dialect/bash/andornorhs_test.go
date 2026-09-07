// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/syntax"
)

// An `&&` or `||` with nothing after it names the token it found rather than
// the operator behind it. The same rule a bar follows since #1115, which fixed
// it for the bar and left the and-or inventing `expected a command after &&` —
// a sentence no shell in the panel writes, blaming a token the parser had
// already read past.
//
// Measured 2026-09-07, bash 5.3.15 and bash 3.2.57 alike, from a script file
// under `env -i` with a scratch HOME:
//
//	$ bash -n s.sh          # echo one && ; echo two
//	s.sh: line 1: syntax error near unexpected token `;'
//	$ bash -n s.sh          # echo a && fi
//	s.sh: line 1: syntax error near unexpected token `fi'
//	$ bash -n s.sh          # { : || \n }
//	s.sh: line 2: syntax error near unexpected token `}'
//	$ bash -n s.sh          # echo one &&\n
//	s.sh: line 2: syntax error: unexpected end of file
//
// The last row is the shape that must *not* take this path: input that ran out
// is unfinished rather than wrong, and this shell says so in a different
// sentence and on the line after the input's last.
func TestAnAndOrWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct {
		src, want string
		line      int
	}{
		{"echo one && ; echo two\n", "syntax error near unexpected token `;'", 1},
		{"echo one || ; echo two\n", "syntax error near unexpected token `;'", 1},
		{"echo one && & echo two\n", "syntax error near unexpected token `&'", 1},
		{"echo one && | echo two\n", "syntax error near unexpected token `|'", 1},
		{"echo one && && echo two\n", "syntax error near unexpected token `&&'", 1},
		{"echo a && fi\n", "syntax error near unexpected token `fi'", 1},
		{"{ : ||\n}\n", "syntax error near unexpected token `}'", 2},
		{"( : ||\n)\n", "syntax error near unexpected token `)'", 2},
		{"while false; do : ||\ndone\n", "syntax error near unexpected token `done'", 2},
		{"echo one &&\n", "syntax error: unexpected end of file", 2},
		{"echo one ||\n", "syntax error: unexpected end of file", 2},
	} {
		_, err := syntax.Parse(tc.src, bash.Dialect())
		if err == nil {
			t.Errorf("%q parsed, want a refusal", tc.src)
			continue
		}
		d := bash.Diagnostics()
		if got := d.ParseFailure(err); got != tc.want {
			t.Errorf("%q:\n got %q\nwant %q", tc.src, got, tc.want)
		}
		if got := d.ParseFailureLine(err); got != tc.line {
			t.Errorf("%q: blamed line %d, want %d", tc.src, got, tc.line)
		}
	}
}

// This shell has no open-ended and-or, which is the other half: the flag is
// zsh's alone and an edit to the preset cannot turn it on here silently.
func TestNoOpenEndedAndOr(t *testing.T) {
	if bash.Dialect().OpenEndedAndOr {
		t.Error("bash refuses an and-or that ends with its operator")
	}
}
