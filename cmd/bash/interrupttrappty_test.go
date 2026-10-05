// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// **A ^C at the prompt runs the INT trap** (#5888).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`: the trap set, `false`,
// `abc`, ^C, then ^U and `$?` with PIPESTATUS on the next line.
//
//	trap                          handler saw   next line
//	trap 'echo …' INT             130           130-130, line given up
//	trap '' INT                   —             1-1, line kept
//
// bash sets the status before the handler runs, so the handler reads 130, and
// gives the line up whatever the handler does. This shell ran no trap at the
// prompt, and under the ignore gave the line up anyway. The zsh twin is in
// cmd/zsh/interrupttrappty_test.go: there the handler decides the line.
func TestControlCAtThePromptRunsTheTrap(t *testing.T) {
	for _, tc := range []struct {
		name, rc string
		saw      string
		givesUp  bool
		want     string
	}{
		{"a trap with a body", "trap 'echo tr-$?-$((6*7))' INT\n", "tr-130-42", true, "130-130"},
		{"trap \"\" INT keeps the line", "trap '' INT\n", "", false, "1-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := interruptSession(t, tc.rc)
			interruptAnswer(t, control, screen, "false", nil)
			if _, err := control.WriteString("abc\x03"); err != nil {
				t.Fatalf("typing abc and ^C: %v", err)
			}
			if tc.saw != "" {
				if err := screen.Await(tc.saw, interruptBudget); err != nil {
					t.Fatalf("the trap did not run, or saw another status: %v", err)
				}
			}
			if tc.givesUp {
				awaitInterruptPrompt(t, screen, "^C")
			}
			got := interruptAnswer(t, control, screen, "\x15"+`echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
			if got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
		})
	}
}
