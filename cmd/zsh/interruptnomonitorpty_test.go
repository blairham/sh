// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"regexp"
	"testing"
)

// **With job control off, a ^C that ends a command ends only that command**
// (#5923).
//
// Measured 2026-10-04 against `/opt/homebrew/bin/zsh` — zsh 5.9.2 — through a
// pseudo-terminal, `-f -i`, `unsetopt monitor`: `/bin/sleep 5`, ^C while it
// runs, then `$?` twice. zsh reads 130 and then 0. This shell printed nothing
// for the first line and 130 for the second: the program ran in the shell's
// own process group, the shell heard the ^C too, and the arrival waited for
// the next line's first command and ended it.
//
// The shell runs in a session of its own (childShellSession), because only
// then is the ^C byte a signal at all. The mark is printed by the program
// itself before it sleeps, so the ^C is sent once a program is running in the
// foreground — a mark printed by the shell would leave a window in which the
// terminal is still the shell's.
func TestControlCWithJobControlOffEndsOnlyThatCommand(t *testing.T) {
	control, screen := childShellSession(t, "unsetopt monitor\n", "-i")
	from := len(screen.Text())
	if _, err := control.WriteString("/bin/sh -c 'echo go-$((6*7)); exec /bin/sleep 5'\n"); err != nil {
		t.Fatalf("typing the command: %v", err)
	}
	interruptDrawnSince(t, screen, from, regexp.MustCompile(`go-(42)`))
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatalf("typing ^C: %v", err)
	}
	for _, mark := range []string{"JNROW", jobNoticeMark} {
		if err := screen.Await(mark, jobNoticeBudget); err != nil {
			t.Fatalf("no prompt after ^C: %v", err)
		}
	}
	if got := interruptAnswer(t, control, screen, "print -r -- st-$?-", interruptStatusLine); got != "130-" {
		t.Errorf("the line after the ^C read %q, want %q", got, "130-")
	}
	if got := interruptAnswer(t, control, screen, "print -r -- st-$?-", interruptStatusLine); got != "0-" {
		t.Errorf("and the line after it read %q, want %q", got, "0-")
	}
}
