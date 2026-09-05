// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// A bare `exit` in an EXIT trap reports the status the shell had when the trap
// began, not that of the trap's own last command.
//
// Found by `make wild-run`: /usr/bin/bzless traps `stty …; exit` on EXIT, and
// with no terminal the `stty` fails — so the script exited 1 where every shell
// exits 0. The output was identical, which is what made it worth running the
// scripts rather than only parsing them.
func TestABareExitInAnExitTrap(t *testing.T) {
	for _, c := range []struct {
		name    string
		earlier Answer
		src     string
		want    int
	}{
		// The status from before the trap, which is three of the four.
		{"keeps the earlier status", Yes, `trap "false; exit" 0; true`, 0},
		{"keeps a status set by exit", Yes, `trap "false; exit" 0; exit 3`, 3},
		{"keeps a failure too", Yes, `trap "true; exit" 0; false`, 1},
		// zsh's answer: whatever the trap's own last command did.
		{"or the trap's own last command", No, `trap "false; exit" 0; true`, 1},
		{"and its success", No, `trap "true; exit" 0; false`, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			earlier := c.earlier
			_, st := run(t, c.src, func(r *Runner) {
				s := *r.Semantics
				s.ExitInTrapReportsEarlierStatus = earlier
				r.Semantics = &s
			})
			if st != c.want {
				t.Errorf("status %d, want %d", st, c.want)
			}
		})
	}
}

// Only the bare form, and only an `exit`. These are unanimous, so the axis
// must not reach them.
func TestWhatTheExitTrapRuleDoesNotTouch(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want int
	}{
		// An explicit status wins, whichever way the axis is set.
		{"an explicit status", `trap "exit 7" 0; true`, 7},
		// A trap that does not exit leaves the script's status alone, so the
		// `false` inside it is invisible either way.
		{"no exit at all", `trap "false" 0; true`, 0},
		{"no exit, after a failure", `trap "true" 0; false`, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			for _, earlier := range []Answer{Yes, No} {
				a := earlier
				_, st := run(t, c.src, func(r *Runner) {
					s := *r.Semantics
					s.ExitInTrapReportsEarlierStatus = a
					r.Semantics = &s
				})
				if st != c.want {
					t.Errorf("with the axis %v: status %d, want %d", a, st, c.want)
				}
			}
		})
	}
}

// An `exit` raised inside a signal handler ends the shell from wherever it was
// raised, and a loop that was running when the signal arrived is not an
// exception.
//
// The status is the whole of it: the handler's output was there and `after`
// was unreached even while this was wrong, so the only thing a caller could
// see was a 0 where every shell in the panel leaves 7. Unanimous across bash
// 5.3.15, bash 3.2.57, that 5.3.15 build named `sh`, dash, ksh93u+ and zsh
// 5.9.2,
// measured 2026-09-05 — a core answer rather than an axis.
//
// The shape that discriminates is `while`, and the mechanism says why. A
// handler runs between commands, so the `exit` is read at the top of the
// command *after* the one the signal interrupted — which in a conditional
// loop is the next round's condition. A loop that reads that refusal as
// though it were the condition's answer finds it non-zero, concludes the loop
// is over, and puts its own bookkeeping over the top of the 7. A `for` reads
// no condition, has nothing to mistake it for, and was right throughout.
//
// The loops are bounded rather than `while :`, so a regression is a failing
// status rather than a test that never returns.
func TestAnExitFromASignalHandlerEndsTheShellFromInsideALoop(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{
			"a while loop",
			`trap 'echo caught; exit 7' USR1; i=0; while [ $i -lt 3 ]; do i=$((i+1)); kill -USR1 $$; done; echo after`,
		},
		{
			// The other way of reading the same status, so a shell that
			// mistakes the refusal for a condition gets the opposite wrong
			// answer here: it runs the body again rather than stopping.
			"an until loop",
			`trap 'echo caught; exit 7' USR1; i=0; until [ $i -ge 3 ]; do i=$((i+1)); kill -USR1 $$; done; echo after`,
		},
		{
			"a for loop",
			`trap 'echo caught; exit 7' USR1; for i in 1 2 3; do kill -USR1 $$; done; echo after`,
		},
		{
			"a C-style for loop",
			`trap 'echo caught; exit 7' USR1; for ((i=0;i<3;i++)); do kill -USR1 $$; done; echo after`,
		},
		{
			// Two levels, because recovering at one of them still leaves the
			// outer loop to go round and report its own bookkeeping.
			"nested while loops",
			`trap 'echo caught; exit 7' USR1; i=0; while [ $i -lt 2 ]; do j=0; while [ $j -lt 2 ]; do j=$((j+1)); kill -USR1 $$; done; i=$((i+1)); done; echo after`,
		},
		{
			// An `exit` is not a `return`, so the call does not absorb it.
			"a loop inside a function",
			`trap 'echo caught; exit 7' USR1; f() { i=0; while [ $i -lt 3 ]; do i=$((i+1)); kill -USR1 $$; done; }; f; echo after`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runGrammar(t, c.src, func(d *syntax.Dialect) { d.CStyleFor = true }, nil)
			if st != 7 {
				t.Errorf("status %d, want the 7 the handler exits with", st)
			}
			if out != "caught\n" {
				t.Errorf("wrote %q, want %q — the handler's line once and nothing after the loop", out, "caught\n")
			}
		})
	}
}
