// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/syntax"
)

// A background job whose body neither reads nor runs a program is the one
// shape the four settling triggers cannot reach, and it left the shell that
// started it waiting forever (#1283).
//
// Bounded for the reason blockingread_test.go states: the bug is a shell that
// never returns, and a test that simply called Run would hang the suite rather
// than fail it. runBoundedScript is that bound.
//
// Every loop here is one the *shell* has to escape before the loop can end —
// it spins on a file the script creates after the `&` — so the assertion is
// not that the output arrived but that the shell got past the job at all. It
// is the same shape the fifo cases use, and it also keeps the suite from
// leaving a goroutine spinning on a core for the rest of the run.
func TestABackgroundJobSpinningOnNothingDoesNotBlockTheShell(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		enable    func(*syntax.Dialect)
	}{
		{
			name: "while",
			src:  "{ while [ ! -f go ]; do :; done; echo LATE; } & echo AFTER; : > go; wait",
		},
		{
			name: "until",
			src:  "{ until [ -f go ]; do :; done; echo LATE; } & echo AFTER; : > go; wait",
		},
		{
			// The loop with no condition at all, which reaches the same back
			// edge through a different clause.
			name: "for-arith",
			src:  "for ((;;)); do [ -f go ] && break; done & echo AFTER; : > go; wait; echo LATE",
			enable: func(d *syntax.Dialect) {
				d.CStyleFor = true
			},
		},
		{
			// Through a function, so the trigger is about where the loop runs
			// and not about the statement `&` was written on.
			name: "in a function",
			src:  "f() { while [ ! -f go ]; do :; done; echo LATE; }; f & echo AFTER; : > go; wait",
		},
		{
			// And in a subshell, which runs the body on a further clone: the
			// job the trigger has to find is the one `&` started, not the
			// runner the loop happens to be on.
			name: "in a subshell",
			src:  "( while [ ! -f go ]; do :; done; echo LATE ) & echo AFTER; : > go; wait",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBoundedScript(t, tc.src, tc.enable, nil)
			if !strings.HasPrefix(out, "AFTER\n") {
				t.Errorf("got %q status %d, want the shell to reach the next command", out, st)
			}
			if !strings.Contains(out, "LATE") {
				t.Errorf("got %q, want the job to have finished its loop", out)
			}
		})
	}
}

// The answers the trigger must not cost, which is what makes it the back edge
// of an unbounded loop rather than the start of a body.
//
// `$!` is a job's process where the job has one, and settling the moment a
// body began would have made every one of these read 0.
func TestABackgroundJobStillReportsTheProcessItStarts(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		// The most ordinary shape there is, and the one #1283 named as the
		// answer a start-of-body latch would lose.
		{"a command after a builtin", "{ echo hi >/dev/null; sleep 0.1; } & echo \"$!\""},
		// A `for` over a word list knows how many passes it has before the
		// first one, so it is not an unbounded loop and does not settle.
		{"after a word-list loop", "{ for i in 1 2 3; do :; done; sleep 0.1; } & echo \"$!\""},
		// And an unbounded loop whose body runs a program settles on that
		// program, because the process starts before the back edge is reached.
		{"a loop whose body runs a program", "{ while :; do sleep 0.1; break; done; } & echo \"$!\""},
		// The back edge is the one a loop about to take *another* pass
		// reaches, which is what puts the trigger after the break is
		// consumed rather than before it: a loop that leaves on its first
		// pass is bounded, and the program after it is still the job's.
		{"a loop that breaks on its first pass", "{ while :; do break; done; sleep 0.1; } & echo \"$!\""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runBoundedScript(t, tc.src, nil, nil)
			if got := strings.TrimSpace(out); got == "0" || got == "" {
				t.Errorf("$! = %q status %d, want the job's process", got, st)
			}
		})
	}
}

// And the answer it does cost, stated rather than left to be discovered: a
// bounded computation written as an unbounded loop settles at its first back
// edge, so `$!` is 0 where it was the sleep's process.
//
// The trade #1283 weighed, paid on the rarer shape. A hang has no status to
// check and no diagnostic to read; a job reported as having no process of its
// own is the answer this shell already gives for every background builtin.
func TestAConditionalLoopSettlesTheJobEvenWhenItWouldHaveEnded(t *testing.T) {
	const src = `{ i=0; while [ $i -lt 1 ]; do i=1; done; sleep 0.1; } & echo "$!"`
	out, st := runBoundedScript(t, src, nil, nil)
	if got := strings.TrimSpace(out); got != "0" {
		t.Errorf("$! = %q status %d, want 0: the job settled at the loop's back edge", got, st)
	}
}
