// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real zsh draws <$LOGNAME>@ —
// the four characters as they stand. It wants `setopt PROMPT_SUBST` before it
// will expand a prompt, which is why this is the one dialect here that says
// no.

func TestPromptStyle(t *testing.T) {
	if got := zsh.PromptStyle().Expand; got != false {
		t.Errorf("Expand = %v, want %v", got, false)
	}
}
