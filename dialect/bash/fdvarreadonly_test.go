// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A frozen name does **not** stop the close of the descriptor it holds here,
// which is the No side of interp.Semantics.ReadonlyFdVariableRefusesAClose.
//
// The row exists because a mutation that made the refusal unconditional —
// ignoring the axis and refusing in every dialect — survived a suite that
// graded only the two columns answering Yes. An axis needs a row on each side
// or it is a constant with a comment beside it.
//
// Measured 2026-09-29 on bash 5.3.20 (`/opt/homebrew/bin/bash`), a script
// file:
//
//	exec {m}>f; readonly m; exec {m}>&-; echo "after=$?"
//	after=0, nothing on standard error
//
// zsh and ksh93u+ refuse the same text. What makes a shell free to allow it
// is that the close **reads** the name rather than writing it: a successful
// `exec {m}>&-` leaves `m` holding the number it held, here as well.
func TestAFrozenNameDoesNotStopTheClose(t *testing.T) {
	out, errs := runBashSplit(t, "exec {m}>f\nreadonly m\nexec {m}>&-\necho \"after=$?\"\n")
	if out != "after=0\n" || errs != "" {
		t.Errorf("out %q err %q, want the close to happen in silence at 0", out, errs)
	}
}

// The opening half is refused in every column that has the grammar, and here
// it is refused in **two** sentences: the store's own, and then the one that
// says the descriptor could not be stored. That pairing is what the zsh
// wording replaces rather than joins, so it is worth a row that would notice
// if the new field started being written here.
func TestTheOpeningHalfIsStillRefusedInTwoSentences(t *testing.T) {
	out, errs := runBashSplit(t, "readonly m\necho x {m}>f\necho \"after=$?\"\n")
	if out != "after=1\n" {
		t.Errorf("out %q, want the command not run and 1", out)
	}
	if want := "bash: line 2: m: readonly variable\nbash: line 2: m: cannot assign fd to variable\n"; errs != want {
		t.Errorf("said %q, want %q", errs, want)
	}
}
