// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
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

// The table, as measured against real zsh.
func TestPromptCodes(t *testing.T) {
	st := zsh.PromptStyle()
	if st.Escape != '%' {
		t.Errorf("Escape = %q, want a percent sign", st.Escape)
	}
	if st.Unknown != repl.DropBoth {
		t.Errorf("Unknown = %v, want an unknown code removed", st.Unknown)
	}
	if st.Privilege != "%" {
		t.Errorf("Privilege = %q, want %%", st.Privilege)
	}
	for code, want := range map[rune]repl.PromptField{
		'n': repl.FieldUser, 'm': repl.FieldHost, 'M': repl.FieldHostFull,
		'~': repl.FieldCwd, 'd': repl.FieldCwdFull, 'C': repl.FieldCwdBase,
		'#': repl.FieldPrivilege, '%': repl.FieldEscape,
		't': repl.FieldTime12Padded, '*': repl.FieldTime24, 'w': repl.FieldDateShort,
	} {
		if got := st.Codes[code]; got != want {
			t.Errorf("%%%c drew field %v, want %v", code, got, want)
		}
	}
}
