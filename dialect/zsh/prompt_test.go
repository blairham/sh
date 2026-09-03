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

// zsh's defaults, as measured — including a continuation written the way zsh
// writes it, with a code that is not drawable yet.
func TestPromptDefaults(t *testing.T) {
	st := zsh.PromptStyle()
	if st.Default != "%m%# " {
		t.Errorf("default = %q, want %%m%%# — host and privilege", st.Default)
	}
	if st.DefaultContinued != "%_> " {
		t.Errorf("continuation = %q, want %%_> ", st.DefaultContinued)
	}
	// `%_` draws what the line is still inside, which is what makes the
	// default continuation say `for> ` inside a for loop.
	if st.Codes['_'] != repl.FieldOpenState {
		t.Errorf("%%_ draws %v, want the open state", st.Codes['_'])
	}
}

// A bare `!` is a bare `!` here: measured, only ksh93 reads it.
func TestNoHistoryCharacter(t *testing.T) {
	if got := zsh.PromptStyle().History; got != 0 {
		t.Errorf("History = %q, want none", got)
	}
}
