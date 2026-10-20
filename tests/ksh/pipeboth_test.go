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
// are not two values of one flag, and this dialect reads the operator through
// Dialect.CoprocPipeOperator with PipeBothStreams left off (#1141).
//
// The assertion that `a |& b` is *refused* stood here until the construct
// existed, and is replaced rather than deleted: what it was really pinning is
// that the two flags never both apply, which is now asked directly.
func TestNoPipeBothStreams(t *testing.T) {
	d := ksh.Dialect()
	if d.PipeBothStreams {
		t.Error("ksh93's `|&` is a coprocess, not a pipe of both streams")
	}
	if !d.CoprocPipeOperator {
		t.Error("ksh93's `|&` is a coprocess and this dialect should read one")
	}
	// The command after it is a command of its own rather than the other
	// half of a pipe, which is what the flag means: `echo one |& echo two`
	// prints only `two`, `one` having gone into the coprocess pipe.
	f, err := syntax.Parse("echo one |& echo two\n", d)
	if err != nil {
		t.Fatalf("`echo one |& echo two`: %v", err)
	}
	if n := len(f.Stmts); n != 2 {
		t.Fatalf("statements = %d, want 2 — the operator terminates rather than joins", n)
	}
	if !f.Stmts[0].Coprocess || !f.Stmts[0].Background {
		t.Errorf("first statement = %+v, want a backgrounded coprocess", f.Stmts[0])
	}
	if f.Stmts[1].Coprocess {
		t.Error("the second statement is an ordinary one, not a coprocess")
	}
	// And a pipe still needs its right-hand command, which is the row that
	// keeps the flag from having been read as "a bar may end a command".
	if _, err := syntax.Parse("echo one | echo two |\n", d); err == nil {
		t.Error("`echo one | echo two |` parsed, want a bar with no command refused")
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
//
// The `|& echo two` row has left this list: the operator is a coprocess here
// now and the line parses, which is #1141 and is asserted above instead.
func TestABarWithNoCommandNamesWhatItFound(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo one | ; echo two\n", "syntax error at line 1: `;' unexpected"},
		{"echo one | & echo two\n", "syntax error at line 1: `&' unexpected"},

		{"echo one | | echo two\n", "syntax error at line 1: `|' unexpected"},
		{"echo one |\n", "syntax error at line 2: `end of file' unexpected"},
		// The whole operator, because the shell lexes it as one token —
		// re-measured 2026-09-07 now that this dialect does too: `|& cat`
		// and a bare `|&` are both ``syntax error at line 1: `|&'
		// unexpected`` there, where `| cat` names the bar alone. The `|`
		// this row wanted before was the fallback lexing of a dialect
		// without the operator, which is our answer and was never ksh's.
		{"|& cat\n", "syntax error at line 1: `|&' unexpected"},
		{"|&\n", "syntax error at line 1: `|&' unexpected"},
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
