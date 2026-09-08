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

// dash is one of the two columns that assigns a prompt to a shell with nobody
// to prompt. Measured on `-c` and on a script file alike with nothing
// inherited: PS1 is `$ ` and PS2 is `> `, the same text a person gets — where
// bash 5.3.15, bash 3.2.57, bash under argv[0] `sh` and ksh93 all leave PS1
// unset, and zsh 5.9.2 sets it to the empty string.
//
// It matters because that is the other side of `[ -z "$PS1" ] && return`: a
// shell that assigned a prompt where the panel leaves none would stop the
// guard ever firing, which is the same defect as an empty PS1 read from the
// other direction (#1421).
func TestAScriptGetsAPromptToo(t *testing.T) {
	st := dash.PromptStyle()
	if !st.AssignsWithNobodyToPrompt {
		t.Error("dash sets PS1 and PS2 in a non-interactive shell")
	}
	if st.DefaultWithNobodyToPrompt != "$ " || st.DefaultContinuedWithNobodyToPrompt != "> " {
		t.Errorf("non-interactive prompts = %q/%q, want %q/%q",
			st.DefaultWithNobodyToPrompt, st.DefaultContinuedWithNobodyToPrompt, "$ ", "> ")
	}
}
