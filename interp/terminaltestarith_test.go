// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// `-t`'s operand read as an arithmetic **expression**, in the one dialect that
// reads it that way, and in every construct that has the operator.
//
// Measured 2026-09-28 against `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `go version -m` *not a Go executable*, from
// script files under `env -i PATH=/usr/bin:/bin` with a scratch `HOME` and
// standard input on `/dev/null`.
//
// **Almost every `-t` row is worthless as evidence and the issue said so.**
// With no descriptor open on a terminal, `[[ -t 0 ]]`, `[[ -t 9 ]]`,
// `[[ -t abc ]]`, `[[ -t 1+1 ]]`, `[[ -t n ]]` and `[[ -t 0x1 ]]` all answer 1
// whether the operand was evaluated or read as a literal. A grid of those says
// the two shells agree and shows nothing. They are kept below, named as the
// non-evidence they are, so that nobody re-derives the rule from them.
//
// Two kinds of row do discriminate without a terminal:
//
//   - an expression that will **not parse**, which is fatal here;
//   - an expression with a **side effect**, which is the one that says the
//     operand is evaluated rather than merely parsed more leniently.
func TestTheTerminalTestOperandIsAnArithmeticExpression(t *testing.T) {
	zshish := func(r *Runner) {
		sem := CoreSemantics()
		sem.TerminalTestOperandIsArithmetic = Yes
		sem.ConditionArithmeticErrorIsFatal = Yes
		r.Semantics, r.Diagnostics = &sem, &Diagnostics{}
	}
	t.Run("a side effect in the operand really happens", func(t *testing.T) {
		// The row the axis is pinned on, because it needs no terminal and no
		// failure: if the operand were read as a literal descriptor, `x`
		// could not move.
		out, _ := run(t, `x=0; [[ -t x++ ]]; echo "x=$x"`, zshish)
		if !strings.Contains(out, "x=1") {
			t.Errorf("out = %q, want the operand evaluated and x moved to 1", out)
		}
	})
	t.Run("and an expression that will not parse ends the script", func(t *testing.T) {
		out, _ := run(t, `[[ -t / ]]; echo st=$?; echo after`, zshish)
		if strings.Contains(out, "after") {
			t.Errorf("out = %q, want the script to end at the math failure", out)
		}
		if !strings.Contains(out, "/") {
			t.Errorf("out = %q, want the operand named in the complaint", out)
		}
	})
	t.Run("while a dialect that has not taken it reads a literal descriptor", func(t *testing.T) {
		// The zero value, and the row that says this is an axis rather than a
		// correction: `x` does not move and the script runs on.
		out, _ := run(t, `x=0; [[ -t x++ ]]; echo "x=$x after"`, func(r *Runner) {
			sem := CoreSemantics()
			sem.TerminalTestRequiresANumber = No
			r.Semantics, r.Diagnostics = &sem, &Diagnostics{}
		})
		if !strings.Contains(out, "x=0 after") {
			t.Errorf("out = %q, want the operand left alone and the script running on", out)
		}
	})
	// The six rows that agree either way. They are here to stay agreeing —
	// a change that broke them would be caught — and explicitly not as
	// evidence for the rule.
	t.Run("the non-discriminating rows still answer 1", func(t *testing.T) {
		for _, src := range []string{
			`[[ -t 0 ]]; echo st=$?`, `[[ -t 9 ]]; echo st=$?`,
			`[[ -t abc ]]; echo st=$?`, `[[ -t 1+1 ]]; echo st=$?`,
			`[[ -t n ]]; echo st=$?`, `[[ -t 0x1 ]]; echo st=$?`,
		} {
			if out, _ := run(t, src, zshish); !strings.Contains(out, "st=1") {
				t.Errorf("%s = %q, want st=1", src, out)
			}
		}
	})
}

// And it is the **operator's** answer rather than `[[ ]]`'s: `test -t` and
// `[ -t ]` read the operand the same way and end the same way.
//
// That is measured and not assumed, and it is what keeps this off
// TestBuiltinComparisonOperandsAreArithmetic's route: in the same shell
// `test -t /` writes `bad math expression` with **no builtin name** and ends
// the script, where `test 1 -eq /` writes `test:1: integer expression
// expected: /` at 2 and the script runs on. A fix that put `-t` on the
// comparison's route would have worded two of the three surfaces wrongly.
func TestTheBuiltinsReadTheTerminalTestOperandTheSameWay(t *testing.T) {
	zshish := func(r *Runner) {
		sem := CoreSemantics()
		sem.TerminalTestOperandIsArithmetic = Yes
		sem.ConditionArithmeticErrorIsFatal = Yes
		r.Semantics, r.Diagnostics = &sem, &Diagnostics{}
	}
	for _, src := range []string{
		`test -t / ; echo st=$?; echo after`,
		`[ -t / ]; echo st=$?; echo after`,
	} {
		t.Run(src, func(t *testing.T) {
			out, status := run(t, src, zshish)
			if strings.Contains(out, "after") {
				t.Errorf("out = %q, want the script to end", out)
			}
			// **No builtin name in front**, which is the half that is its own
			// route rather than the comparison's.
			if strings.Contains(out, "test:") || strings.Contains(out, "[:") {
				t.Errorf("out = %q, want the complaint as the shell's rather than the builtin's", out)
			}
			if status != 1 {
				t.Errorf("status = %d, want 1", status)
			}
		})
	}
	t.Run("and a side effect happens there too", func(t *testing.T) {
		out, _ := run(t, `x=0; test -t x++ ; echo "x=$x"`, zshish)
		if !strings.Contains(out, "x=1") {
			t.Errorf("out = %q, want the operand evaluated", out)
		}
	})
}
