// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// This shell reads `-t`'s operand as an arithmetic expression, in every
// construct that has the operator (#5058).
//
// Graded here as well as in `interp` because two halves of it are the
// *dialect's* and the substrate's tests cannot see them: whether this column
// takes the axis at all, and whether the complaint carries the builtin's name.
// A core runner names no builtin in a location whatever the code does, so a
// mutation that put `test:` back in front killed nothing until this file
// existed.
//
// Measured 2026-09-28 against zsh 5.9.2 (aarch64-apple-darwin25.4.0),
// `go version -m` *not a Go executable*, script files under `env -i
// PATH=/usr/bin:/bin` with standard input on `/dev/null`.
func TestThisShellReadsTheTerminalTestOperandAsArithmetic(t *testing.T) {
	if got := zsh.Semantics().TerminalTestOperandIsArithmetic; got != interp.Yes {
		t.Fatalf("TerminalTestOperandIsArithmetic is %v, want Yes", got)
	}
	t.Run("the operand is evaluated, side effects and all", func(t *testing.T) {
		// The row that needs no terminal and no failure: a literal descriptor
		// could not move `x`.
		out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{}, `x=0; [[ -t x++ ]]; print -r -- "x=$x"`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "x=1") {
			t.Errorf("out = %q, want x moved to 1", out)
		}
	})
	for _, tc := range []struct{ name, src string }{
		{"inside the brackets", `[[ -t / ]]; print -r -- after`},
		{"in the test builtin", `test -t / ; print -r -- after`},
		{"and in the bracket builtin", `[ -t / ]; print -r -- after`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{}, tc.src)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, "bad math expression") {
				t.Errorf("out = %q, want the math complaint", out)
			}
			if strings.Contains(out, "after") {
				t.Errorf("out = %q, want the script to end at the failure", out)
			}
			// **No builtin name in the location**, which is what keeps `-t`
			// off the comparison's route: `test 1 -eq /` in this shell *does*
			// carry it — `zsh:test:N: integer expression expected: /` — and
			// the row below is that contrast.
			if strings.Contains(out, "test:") || strings.Contains(out, ":[:") {
				t.Errorf("out = %q, want the complaint as the shell's", out)
			}
		})
	}
	t.Run("while the comparison keeps the builtin's name and runs on", func(t *testing.T) {
		out, _, err := preset.CombinedWithPrelude(t, dialecttest.Base{}, `test 1 -eq / ; print -r -- after`)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "test:") {
			t.Errorf("out = %q, want the builtin named", out)
		}
		if !strings.Contains(out, "after") {
			t.Errorf("out = %q, want the script to run on", out)
		}
	})
}
