// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ash"
)

// `^C` after an abandoned line. A shell that takes the terminal raw has to
// draw it to look like one that lets the terminal echo it.
func TestEditorStyleAnswers(t *testing.T) {
	if got := ash.EditorStyle().Interrupt; got != "^C" {
		t.Errorf("Interrupt = %q, want ^C", got)
	}
}

// The zero value, and here it means "no answer" rather than "ash's answer":
// BusyBox builds its line editing and history in or out, so there is no one
// behavior to be compatible with — and with no ash on this machine there is no
// way to measure which build a reader has.
func TestHistoryStyleIsDeliberatelyEmpty(t *testing.T) {
	if got := ash.HistoryStyle(); got.SearchPrompt != "" {
		t.Errorf("HistoryStyle = %+v, want the zero value", got)
	}
}
