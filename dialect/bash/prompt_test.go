// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/repl"
)

// // Measured: with PS1='<$LOGNAME>@ ' exported, real bash draws <bhamilton>@,
// and with PS1='<$(echo sub)>@ ' it draws <sub>@ — expanded, and expanded
// again at every prompt.

func TestPromptStyle(t *testing.T) {
	if got := bash.PromptStyle().Expand; got != true {
		t.Errorf("Expand = %v, want %v", got, true)
	}
}

// The table, as measured. Each of these was drawn by real bash at a prompt
// with PS1 set to that one code.
func TestPromptCodes(t *testing.T) {
	st := bash.PromptStyle()
	if st.Escape != '\\' {
		t.Errorf("Escape = %q, want a backslash", st.Escape)
	}
	if st.Unknown != repl.KeepBoth {
		t.Errorf("Unknown = %v, want an unknown code kept whole", st.Unknown)
	}
	if st.Privilege != "$" {
		t.Errorf("Privilege = %q, want $", st.Privilege)
	}
	for code, want := range map[rune]repl.PromptField{
		'u': repl.FieldUser, 'h': repl.FieldHost, 'H': repl.FieldHostFull,
		'w': repl.FieldCwd, 'W': repl.FieldCwdBase, 's': repl.FieldShellName,
		'$': repl.FieldPrivilege, 't': repl.FieldTime24, 'T': repl.FieldTime12,
		'A': repl.FieldTime24HM, '@': repl.FieldTime12AMPM, 'd': repl.FieldDate,
		'\\': repl.FieldEscape,
	} {
		if got := st.Codes[code]; got != want {
			t.Errorf("\\%c drew field %v, want %v", code, got, want)
		}
	}
	// Not a tab. bash's \t is the time, and nothing carries over from C.
	if st.Codes['t'] == repl.FieldTab {
		t.Error("\\t is the time in bash, not a tab")
	}
}
