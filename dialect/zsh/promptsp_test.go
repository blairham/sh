// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// What this shell does about output that never ended its line.
//
// Measured 2026-09-12 through a pseudo-terminal with an rc file ending
// `printf 'LEFTOVER'`: a bold, inverse `%` where the output stopped, padded to
// the end of the row so the terminal wraps, and the prompt on the row below —
// where bash draws its prompt straight onto the output.
//
// Three answers, and they are asserted here because repl names no shell and a
// dialect that gave none would leave the whole thing off with nothing failing.
func TestThisShellMarksUnfinishedOutput(t *testing.T) {
	style := zsh.EditorStyle()
	if style.MarkUnfinishedOutputOption != "PROMPT_SP" {
		t.Errorf("marking option = %q, want PROMPT_SP", style.MarkUnfinishedOutputOption)
	}
	if style.ReturnBeforeThePromptOption != "PROMPT_CR" {
		t.Errorf("return option = %q, want PROMPT_CR", style.ReturnBeforeThePromptOption)
	}
	if !style.ClearsBelowThePrompt {
		t.Error("the rows below the prompt are not cleared; measured, this shell clears them")
	}
	// The mark is an inverse `%`, and it has to carry its own reset or the
	// prompt after it is drawn inverse too.
	if !strings.Contains(style.UnfinishedOutputMark, "%") {
		t.Errorf("mark = %q, want a %% in it", style.UnfinishedOutputMark)
	}
	if !strings.HasSuffix(style.UnfinishedOutputMark, "\x1b[0m") {
		t.Errorf("mark = %q, want it to end by resetting — the prompt follows it",
			style.UnfinishedOutputMark)
	}
	// Both options exist and both are on before anybody changes them, which is
	// what makes the marking this shell's default behavior rather than an
	// opt-in. A name the option table does not carry would read as off.
	for _, opt := range []string{"promptsp", "promptcr"} {
		out, st := runZsh(t, t.TempDir(), "[[ -o "+opt+" ]] && print -r -- on || print -r -- off\n")
		if st != 0 || strings.TrimSpace(out) != "on" {
			t.Errorf("%s = %q status %d, want on", opt, out, st)
		}
	}
}
