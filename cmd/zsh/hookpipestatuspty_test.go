// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// **A prompt hook keeps the pipeline record of the command typed before it**,
// as it already kept `$?` (#5889).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`, after `true | false`:
//
//	precmd                                   the hook b reads   next line
//	precmd() { print -r -- "pc[$?]" }        —                  1 0 1
//	precmd_functions=(a b), a runs
//	  `false | false | false`                1 0 1              1 0 1
//
// This shell put `$?` back after the hooks and left the record as the last
// hook's own commands had made it: `1 0` on the next line, and `0 1 1 1` in
// the second hook. The two-hook row is the one that says the record is put
// back before every hook and not only after the last.
func TestAPromptHookKeepsThePipelineRecord(t *testing.T) {
	rc := "a() { false | false | false }\n" +
		"b() { print -r -- \"pb[$? ${pipestatus[*]}]\" }\n" +
		"precmd_functions=(a b)\n"
	control, screen, _ := jobNoticeSessionRC(t, rc, "zsh", "-i")
	hook := regexp.MustCompile(`pb\[([0-9][0-9 ]*)\]`)
	from := len(screen.Text())
	interruptType(t, control, screen, "true | false")
	if got := interruptDrawnSince(t, screen, from, hook); got != "1 0 1" {
		t.Errorf("the second hook read %q, want %q", got, "1 0 1")
	}
	if got := interruptAnswer(t, control, screen, interruptRecordProbe, interruptStatusLine); got != "1-0 1" {
		t.Errorf("the line after the hooks read %q, want %q", got, "1-0 1")
	}
}
