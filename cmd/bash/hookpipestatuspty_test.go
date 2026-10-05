// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// **PROMPT_COMMAND keeps the pipeline record of the command typed before it**,
// as it already kept `$?` (#5889).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`, after `true | false`,
// with PROMPT_COMMAND an array whose first element runs
// `false | false | false` and whose second prints `$?` and PIPESTATUS: the
// second reads `1 0 1`, and so does the next typed line. This shell left the
// first element's record behind for both. The zsh twin is in
// cmd/zsh/hookpipestatuspty_test.go.
func TestAPromptHookKeepsThePipelineRecord(t *testing.T) {
	rc := "PROMPT_COMMAND=('false | false | false' 'echo \"pb[$? ${PIPESTATUS[*]}]\"')\n"
	control, screen := interruptSession(t, rc)
	hook := regexp.MustCompile(`pb\[([0-9][0-9 ]*)\]`)
	from := len(screen.Text())
	interruptAnswer(t, control, screen, "true | false", nil)
	if got := interruptDrawnSince(t, screen, from, hook); got != "1 0 1" {
		t.Errorf("the second element read %q, want %q", got, "1 0 1")
	}
	got := interruptAnswer(t, control, screen, `echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
	if got != "1-0 1" {
		t.Errorf("the line after PROMPT_COMMAND read %q, want %q", got, "1-0 1")
	}
}
