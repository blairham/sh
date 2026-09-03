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

// The question this shell asks before printing a large listing, and how it
// reads the answer. Measured under a pty.
func TestEditorStyleAsksBeforeALargeListing(t *testing.T) {
	s := bash.EditorStyle()
	if got, want := s.ListQuery, "Display all %[1]d possibilities? (y or n)"; got != want {
		t.Errorf("ListQuery = %q, want %q", got, want)
	}
	if !s.ListQueryAcceptsOnlyYesOrNo {
		t.Error("this shell rings the bell at anything that is not y or n and asks again")
	}
	if s.ListQueryEchoesTheKey {
		t.Error("this shell does not write the answering key back")
	}
}
