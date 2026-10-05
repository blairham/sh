// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// **A ^C at the prompt runs the INT trap** (#5888).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`: the trap set, `false`,
// a half-typed line, ^C, and `$?` with PIPESTATUS on the line after.
//
//	trap                          handler saw   then
//	trap 'echo …' INT             130           130-130, line given up
//	trap '' INT                   —             line kept
//
// bash sets the status before the handler runs, so the handler reads 130, and
// gives the line up whatever the handler does. This shell ran no trap at the
// prompt, and under the ignore gave the line up anyway. The zsh twin is in
// cmd/zsh/interrupttrappty_test.go: there the handler decides the line.
func TestControlCAtThePromptRunsTheTrap(t *testing.T) {
	interruptKeptLine := regexp.MustCompile(`kept-([0-9]+)\r?\n`)
	for _, tc := range []struct {
		name, rc string
		saw      string
		givesUp  bool
		want     string
	}{
		{"a trap with a body", "trap 'echo tr-$?-$((6*7))' INT\n", "tr-130-42", true, "130-130"},
		{"trap \"\" INT keeps the line", "trap '' INT\n", "", false, "0-0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := interruptSession(t, tc.rc)
			interruptAnswer(t, control, screen, "false", nil)
			// A kept line is told from a given-up one by what it prints:
			// the front of a command before the ^C, the rest after it. See
			// the zsh twin.
			before := "abc"
			if !tc.givesUp {
				before = "echo kept-"
			}
			if _, err := control.WriteString(before + "\x03"); err != nil {
				t.Fatalf("typing %q and ^C: %v", before, err)
			}
			if tc.saw != "" {
				if err := screen.Await(tc.saw, interruptBudget); err != nil {
					t.Fatalf("the trap did not run, or saw another status: %v", err)
				}
			}
			if tc.givesUp {
				awaitInterruptPrompt(t, screen, "^C")
			} else if got := interruptAnswer(t, control, screen, "$((6*7))", interruptKeptLine); got != "42" {
				t.Fatalf("the line was not kept: it printed %q", got)
			}
			got := interruptAnswer(t, control, screen, "\x15"+`echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
			if got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
		})
	}
}
