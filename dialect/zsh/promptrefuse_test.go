// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
)

// This shell is the one that keeps asking after a construct it has refused.
//
// Measured 2026-09-11, `printf 'echo one\nif; then\necho three\n'` into each
// shell under `-i` with PS1 and PS2 set: bash 5.3.15, ksh93u+ and dash each
// write their syntax error without drawing a continuation prompt and then run
// `echo three`, while this one draws PS2 twice and swallows the third line,
// reporting only at the end of the input.
//
// An answer rather than a bug on its side: it is what the shell does, and the
// cost of giving its answer to the other three was a typed command
// disappearing (#1893).
func TestAPromptKeepsAskingAfterARefusedToken(t *testing.T) {
	if !zsh.Semantics().PromptAsksAgainAfterARefusedToken {
		t.Error("this shell refuses a construct at the prompt, where it waits for another line")
	}
}
