// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// Measured with the output drained after every keystroke — without that the
// terminal flushes what it has queued when the interrupt arrives, and what
// comes back is a screen that never existed. Here, real zsh draws no mark.
func TestEditorStyle(t *testing.T) {
	const want = ""
	if got := zsh.EditorStyle().Interrupt; got != want {
		t.Errorf("Interrupt = %q, want %q", got, want)
	}
}
