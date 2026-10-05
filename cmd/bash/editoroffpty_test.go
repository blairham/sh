// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
)

// **`set +o emacs +o vi` turns the line editor off** (#5922), and a ^C at a
// prompt read without it ends the read at once.
//
// Measured 2026-10-04 against `/opt/homebrew/bin/bash` — GNU bash 5.3.20 —
// through a pseudo-terminal, `--norc --noprofile -i`:
//
//	after set +o emacs +o vi     no `\e[?2004h` around a line; the terminal
//	                             echoes and gathers it
//	abc, ^C                      `^C`, a newline and a fresh prompt at once;
//	                             `$?` 130, PIPESTATUS 130
//	trap 'echo tr-$?' INT,
//	  false, abc, ^C             `tr-1`, the read goes on, `$?` 1
//	trap '' INT, false, abc, ^C  the read goes on, `$?` 1
//	set -o vi afterwards         the editor is back
//
// This shell kept the editor on whatever the modes said. The trap row is the
// one that differs from the same bash with readline, which gives the line up
// and leaves 130 — see interp.Runner.InterruptInATerminalRead.
func TestTheEditingModesTurnTheEditorOff(t *testing.T) {
	t.Run("no editor is drawn, and the modes bring it back", func(t *testing.T) {
		control, screen := interruptSession(t, "set +o emacs +o vi\n")
		from := len(screen.Text())
		interruptAnswer(t, control, screen, "true", nil)
		if strings.Contains(screen.Text()[from:], "\x1b[?2004h") {
			t.Errorf("a prompt with the editor off asked for bracketed paste: %q", screen.Text()[from:])
		}
		interruptAnswer(t, control, screen, "set -o vi", nil)
		from = len(screen.Text())
		interruptAnswer(t, control, screen, "true", nil)
		if !strings.Contains(screen.Text()[from:], "\x1b[?2004h") {
			t.Errorf("`set -o vi` did not bring the editor back: %q", screen.Text()[from:])
		}
	})
	for _, tc := range []struct {
		name, rc string
		// saw is what the trap printed, or empty; givesUp is whether a fresh
		// prompt follows the ^C.
		saw     string
		givesUp bool
		want    string
	}{
		{"with no trap the read ends at the ^C", "", "", true, "130-130"},
		{"a trap keeps the read", "trap 'echo tr-$?-$((6*7))' INT\n", "tr-1-42", false, "1-1"},
		{"an ignore keeps the read", "trap '' INT\n", "", false, "1-1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := interruptSession(t, "set +o emacs +o vi\n"+tc.rc)
			interruptAnswer(t, control, screen, "false", nil)
			from := len(screen.Text())
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
			got := interruptAnswer(t, control, screen, `echo "st-$?-${PIPESTATUS[*]}"`, interruptStatusLine)
			if got != tc.want {
				t.Errorf("after ^C the next line read %q, want %q", got, tc.want)
			}
			// A kept read draws no prompt between the ^C and the line typed
			// after it, which is what tells it from a given-up read whose
			// status the ignore left alone.
			before, _, _ := strings.Cut(screen.Text()[from:], "st-")
			if drew := strings.Contains(before, interruptRow); drew != tc.givesUp {
				t.Errorf("a prompt was drawn after the ^C = %v, want %v: %q", drew, tc.givesUp, before)
			}
		})
	}
}
