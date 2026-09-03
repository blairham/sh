// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/dash"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real dash draws <bhamilton>@,
// and with PS1='<$((1+1))>@ ' it draws <2>@.

func TestPromptStyle(t *testing.T) {
	if got := dash.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

// dash has no escape language at all: `\u` drew `\u`.
func TestPromptCodes(t *testing.T) {
	if st := dash.PromptStyle(); st.Escape != 0 || len(st.Codes) != 0 {
		t.Errorf("Escape = %q with %d codes, want no escape language", st.Escape, len(st.Codes))
	}
}

// A bare `!` is a bare `!` here: measured, only ksh93 reads it.
func TestNoHistoryCharacter(t *testing.T) {
	if got := dash.PromptStyle().History; got != 0 {
		t.Errorf("History = %q, want none", got)
	}
}
