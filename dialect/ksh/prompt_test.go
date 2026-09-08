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

// ksh93 alone reads a bare `!` as the history number.
func TestPromptHistoryCharacter(t *testing.T) {
	if got := ksh.PromptStyle().History; got != '!' {
		t.Errorf("History = %q, want !", got)
	}
}

// ksh93 is the one column in the panel that assigns PS1 *after* its startup
// file rather than before it.
//
// Measured through a pty, `$ENV` naming a file that prints `${PS1+set}`, with
// PS1 unset in the parent: bash 5.3.15, bash 3.2.57, bash under argv[0] `sh`,
// dash and zsh 5.9.2 all have PS1 in hand while that file runs, and ksh93 has
// it unset there — while its PS2 and PS4 are already set. By the time a prompt
// is drawn it reads `$ `, the same value the other timing would have given.
//
// So the disagreement is about the moment and not about the value, which is
// what makes it a row of the prompt table rather than an axis of Semantics.
func TestTheStartupFileDoesNotSeeTheDefaultPrompt(t *testing.T) {
	st := ksh.PromptStyle()
	if !st.DefaultsFollowTheStartupFiles {
		t.Error("ksh93 leaves PS1 unset while $ENV runs and sets it by the time a prompt is drawn")
	}
	if st.Default != "$ " {
		t.Errorf("default prompt = %q, want %q — the value is the same, only the moment differs", st.Default, "$ ")
	}
}
