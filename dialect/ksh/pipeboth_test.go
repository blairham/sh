// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/syntax"
)

// ksh93 spells a *coprocess* with these two characters, which is not this
// construct with another meaning but another slot in the grammar. The
// measurement that separates them is the token allowed after the operator,
// measured on 93u+ 2012-08-01 from a script file under `env -i`:
//
//	$ ksh -n s.sh          # echo one | ; echo two
//	s.sh: syntax error at line 1: `;' unexpected
//	$ ksh -n s.sh          # echo one |& ; echo two
//	                       # accepted
//	$ ksh -n s.sh          # echo one & ; echo two
//	                       # accepted
//
// A `|` there needs a command after it and a `|&` does not, and a bare `&`
// does not either: ksh93's `|&` ends a command the way `&` does rather than
// joining two. A pipe of both streams could not do that, so the two readings
// are not two values of one flag and this dialect does not get PipeBothStreams
// turned on to stand in for a coprocess. The construct ksh93 does have belongs
// beside Dialect.Coproc and is not implemented here yet, so `a |& b` is
// refused rather than run as the wrong thing — which is the refusal a dialect
// without the operator gives, on the `&`.
func TestNoPipeBothStreams(t *testing.T) {
	if ksh.Dialect().PipeBothStreams {
		t.Error("ksh93's `|&` is a coprocess, not a pipe of both streams")
	}
	if _, err := syntax.Parse("echo one |& echo two\n", ksh.Dialect()); err == nil {
		t.Error("`echo one |& echo two` parsed, want a refusal until the coprocess form exists")
	}
}

// The wording half, which is not about `|&` at all: a bar with nothing after
// it names the token it found. This shell quotes it and gives the line, and
// answers the end of input the same way as any other token rather than as a
// construct left open.
//
//	$ ksh -n s.sh          # echo one | ; echo two
//	s.sh: syntax error at line 1: `;' unexpected
//	$ ksh -n s.sh          # echo one |\n
//	s.sh: syntax error at line 2: `end of file' unexpected
//
// All four were `expected a command after |` before #1115 — a sentence no
// shell in the panel writes, blaming a bar that had already been read.
func TestABarWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one | ; echo two\n", "syntax error at line 1: `;' unexpected"},
		{"echo one | & echo two\n", "syntax error at line 1: `&' unexpected"},
		{"echo one |& echo two\n", "syntax error at line 1: `&' unexpected"},
		{"echo one | | echo two\n", "syntax error at line 1: `|' unexpected"},
		{"echo one |\n", "syntax error at line 2: `end of file' unexpected"},
		{"|& cat\n", "syntax error at line 1: `|' unexpected"},
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
