// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/internal/dialecttest"
)

// The prompt language travels with the dialect. #1455.
//
// ksh93's is a language of two rules rather than a table of letters — the
// backslash goes and the letter stands, and a bare `!` is the history number —
// but they are rules a runner has to have been told, and this dialect's Apply
// did not tell it. Only zsh's did.
func TestApplyInstallsThePromptStyle(t *testing.T) {
	preset.PromptTableInstalled(t, dialecttest.Base{Dir: t.TempDir()}, ksh.PromptStyle())
}

// And what it says is not nothing, which is what would make the comparison
// above pass for the wrong reason.
func TestThisDialectDropsTheBackslashAndKeepsTheLetter(t *testing.T) {
	st := ksh.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash", st.Escape)
	}
	if st.History != '!' {
		t.Errorf("History = %q, want `!`: measured, `<!>` drew 1, 2 and 3 on successive prompts", st.History)
	}
}
