// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/repl"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real ksh93 draws <bhamilton>@.
//
// Its other two prompt habits — a bare ! becoming the history number, and a
// backslash being dropped from whatever follows — are separate questions and
// are not answered by this field.

func TestPromptStyle(t *testing.T) {
	if got := ksh.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

// ksh93 has an escape character and no table behind it: the backslash goes
// and the letter stands.
func TestPromptCodes(t *testing.T) {
	st := ksh.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash", st.Escape)
	}
	if st.Unknown != repl.DropEscape {
		t.Errorf("Unknown = %v, want the escape dropped", st.Unknown)
	}
	if len(st.Codes) != 0 {
		t.Errorf("Codes = %v, want none — measured, \\u drew u", st.Codes)
	}
}
