// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// runKshWithTools is runKsh with a PATH that reaches the system's own
// commands, because a coprocess needs a real process on the far end: a
// compound command as the body is a separate gap, and one this shell has
// under `coproc` too.
func runKshWithTools(t *testing.T, src string) (string, int) {
	t.Helper()
	out, st, err := preset.Combined(t, dialecttest.Base{
		Dir:  t.TempDir(),
		Vars: map[string]string{"PATH": "/usr/bin:/bin"},
	}, src)
	if err != nil {
		t.Fatalf("run %q: %v", src, err)
	}
	return out, st
}

// `cmd |&` starts a coprocess, and `print -p` and `read -p` are how a script
// reaches it — this shell publishing no name and no array.
//
// Measured on ksh93u+ 2012-08-01, 2026-09-07, from a script file under
// `env -i`: `cat |&` then `print -p hello` then `read -p line` answers
// `got=[hello]`, and a round trip through a filter answers what the filter
// wrote.
func TestTheCoprocessOperatorStartsOne(t *testing.T) {
	out, st := runKshWithTools(t, "cat |&\nprint -p hello\nread -p line\necho \"got=[$line]\"\n")
	if st != 0 || !strings.Contains(out, "got=[hello]") {
		t.Errorf("got %q (status %d), want got=[hello] at 0", out, st)
	}
}

// The operator terminates the whole and-or, the way `&` does rather than the
// way a pipe binds.
//
// Measured the same day: `echo A && cat |&` puts `echo A`'s output into the
// coprocess pipe, so a `read -p` with nothing written to the coprocess still
// answers `A`. Same for `echo A | cat |&`. That is the row that says the
// operator is a statement terminator and not something attached to the
// command in front of it.
func TestTheCoprocessOperatorTerminatesTheAndOr(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"echo A && cat |&\nread -p l\necho \"read=[$l]\"\n", "read=[A]"},
		{"echo A | cat |&\nread -p l\necho \"read=[$l]\"\n", "read=[A]"},
	} {
		out, st := runKshWithTools(t, tc.src)
		if st != 0 || !strings.Contains(out, tc.want) {
			t.Errorf("%q = %q (status %d), want %q at 0", tc.src, out, st, tc.want)
		}
	}
}

// A second one while the first is still running is refused, and fatally —
// where a second `coproc` in the two shells that have the word replaces its
// predecessor silently.
//
// Measured on ksh93u+, 2026-09-07: two `cat |&` in a row answer
// `process already exists`, the script ends at status 1, and the line after
// the second one is never reached. It is about a coprocess still *running*
// rather than one ever having been started — `true |&`, a wait, then
// `cat |&` is accepted at status 0.
func TestASecondCoprocessWhileOneRunsIsRefused(t *testing.T) {
	out, st := runKshWithTools(t, "cat |&\ncat |&\necho after\n")
	if !strings.Contains(out, "process already exists") {
		t.Errorf("got %q, want ksh93's own words", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("got %q, want the refusal fatal", out)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
	// And one whose coprocess has ended is accepted. `true` exits at once,
	// and the shell waits for the job before the second operator is read.
	out, st = runKshWithTools(t, "true |&\nwait\ncat |&\necho \"second=$?\"\n")
	if st != 0 || !strings.Contains(out, "second=0") {
		t.Errorf("got %q (status %d), want second=0 at 0", out, st)
	}
}

// With none running, `read -p` and `print -p` are still the refusals they
// were: the letters name a coprocess, and the words are this shell's own.
func TestTheCoprocessLettersStillRefuseWithNoneRunning(t *testing.T) {
	out, _ := runKshWithTools(t, `print -p hi; echo "st=$?"`)
	if !strings.Contains(out, "print: no query process") {
		t.Errorf("got %q, want ksh93's words for print with no coprocess", out)
	}
}
