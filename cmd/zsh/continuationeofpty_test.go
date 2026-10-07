// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// `^D` on an empty continuation line is not end of input in zsh (#6242).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a two-row
// prompt: `echo "a`, Return, then `^D` at `dquote> ` writes `\a` and nothing
// else — the word is inside the quote, so the listing finds nothing — and the
// command can still be finished. `setopt ignoreeof` changes none of it. Here
// the shell exited on the `^D`, or under ignoreeof wrote its refusal.
func TestControlDAtAContinuationKeepsReading(t *testing.T) {
	for _, rc := range []string{"", "setopt ignoreeof"} {
		t.Run("rc "+rc, func(t *testing.T) {
			control, screen := widgetSession(t, rc)
			send := func(keys string) {
				t.Helper()
				if _, err := control.WriteString(keys); err != nil {
					t.Fatal(err)
				}
			}
			send("echo \"cont\r")
			if err := screen.Await("dquote> ", widgetBudget); err != nil {
				t.Fatalf("no continuation prompt: %v", err)
			}
			before := len(screen.Text())
			send("\x04")
			deadline := time.Now().Add(widgetBudget)
			for !strings.Contains(screen.Text()[before:], "\a") {
				if time.Now().After(deadline) {
					t.Fatalf("no bell for ^D; it wrote:\n%s", smoke.Readable(screen.Text()[before:]))
				}
				time.Sleep(20 * time.Millisecond)
			}
			if got := screen.Text()[before:]; strings.Contains(got, "exit") {
				t.Errorf("^D refused or ended the session:\n%s", smoke.Readable(got))
			}
			// And the command finishes: the two lines printed, which the
			// echo of what was typed cannot spell, since it has the quotes.
			send("inued\"\r")
			if err := screen.Await("cont\r\ninued\r\n", widgetBudget); err != nil {
				t.Fatalf("the command did not finish: %v\n%s", err, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
		})
	}
}
