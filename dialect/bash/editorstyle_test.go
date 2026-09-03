// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// Measured with the output drained after every keystroke — without that the
// terminal flushes what it has queued when the interrupt arrives, and what
// comes back is a screen that never existed. Here, real bash draws `^C` after the abandoned line.
func TestEditorStyle(t *testing.T) {
	const want = "^C"
	if got := bash.EditorStyle().Interrupt; got != want {
		t.Errorf("Interrupt = %q, want %q", got, want)
	}
}
