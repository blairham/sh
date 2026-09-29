// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A frozen name refuses the close of the descriptor it holds here, as in zsh
// and **unlike bash**, which closes it in silence.
//
// This is one of the two columns that answer
// interp.Semantics.ReadonlyFdVariableRefusesAClose Yes, and it is here rather
// than only in zsh because a mutation that made the refusal unconditional —
// ignoring the axis entirely — survived a suite that graded the Yes column
// alone. An axis with two values needs a row on each side or it is not an
// axis, it is a constant with a comment.
//
// Measured 2026-09-29 on ksh93u+ (`/bin/ksh`), a script file:
//
//	exec {m}>f; readonly m; exec {m}>&-
//	ksh: <file>[3]: m: is read only      and the script ends there
//
// The words are this shell's ordinary readonly refusal rather than a sentence
// about the descriptor, which is what tells it from zsh's `can't close file
// descriptor from readonly parameter m` at the same site.
func TestAFrozenNameRefusesTheCloseHereToo(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "exec {m}>f\nreadonly m\nexec {m}>&-\necho after\n")
	if !strings.Contains(out, "m: is read only") {
		t.Errorf("said %q, want the readonly refusal naming m", out)
	}
	if strings.Contains(out, "after") {
		t.Errorf("said %q, want the script to end at the refusal", out)
	}
	if st == 0 {
		t.Errorf("status %d, want a failure", st)
	}
}

// And a writable name closes without a word, which is the control that keeps
// the rule about the attribute rather than about the spelling.
func TestAWritableNameStillClosesHere(t *testing.T) {
	out, st := runKsh(t, t.TempDir(), "exec {m}>f\nexec {m}>&-\necho \"after=$? m=[$m]\"\n")
	if st != 0 || !strings.Contains(out, "after=0 m=[") {
		t.Errorf("out %q status %d, want the close to succeed and the name to keep its number", out, st)
	}
	if strings.Contains(out, "read only") {
		t.Errorf("said %q, want nothing about readonly", out)
	}
}
