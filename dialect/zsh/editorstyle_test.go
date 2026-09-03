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

// The question this shell asks before printing a large listing. It counts the
// rows as well as the matches, echoes the key, and takes the first key it is
// given as the answer.
func TestEditorStyleAsksBeforeALargeListing(t *testing.T) {
	s := zsh.EditorStyle()
	if got, want := s.ListQuery, "zsh: do you wish to see all %[1]d possibilities (%[2]d lines)? "; got != want {
		t.Errorf("ListQuery = %q, want %q", got, want)
	}
	if !s.ListQueryEchoesTheKey {
		t.Error("this shell writes the answering key back")
	}
	if s.ListQueryAcceptsOnlyYesOrNo {
		t.Error("this shell takes the first key, whatever it is")
	}
}
