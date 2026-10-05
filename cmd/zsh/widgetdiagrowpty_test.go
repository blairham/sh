// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A diagnostic the shell writes while a widget runs gets a row of its own, on
// a real terminal (#6085).
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, a widget
// `w` on `^T` with `ab` typed: before the first diagnostic the shell itself
// writes, the editor ends the line's row, and the prompt is drawn again whole
// — both rows of this two-row one — once the call is over. Fatal or not makes
// no difference; what zsh's forked child writes, a subshell's, and what goes
// somewhere other than the terminal never move the row; and it happens once
// a call. Here the message was written at the cursor, on the line's own row.
func TestADiagnosticFromAWidgetGetsARowOfItsOwn(t *testing.T) {
	for _, row := range []struct {
		name, body, says string
		// fresh is whether the message starts a row of its own.
		fresh bool
		// also is what else must be on the screen after the key.
		also string
	}{
		{"a fatal error", "KEYS=x", "w: read-only variable: KEYS", true, ""},
		{
			"a builtin's complaint the function survives", "cd /nx1; print -n AFTER",
			"w:cd: no such file or directory: /nx1", true, "AFTER",
		},
		{
			"once a call", "cd /nx1; print -n X; cd /nx2",
			"w:cd: no such file or directory: /nx1", true, "Xw:cd: no such file or directory: /nx2",
		},
		{"the child's message", "nosuchcmd_zz", "w: command not found: nosuchcmd_zz", false, ""},
		{"a subshell's", "( cd /nx1 )", "w:cd: no such file or directory: /nx1", false, ""},
		{"the trace, which is not a diagnostic", "setopt xtrace; :; unsetopt xtrace", "+w:0> :", false, ""},
		{"a message sent elsewhere", "{ cd /nx1 } 2>/dev/null; print -n AFTER", "AFTER", false, ""},
		{"a parse error eval reports", "eval 'if'", "(eval):1: parse error near `if'", true, ""},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t, "w() { "+row.body+" }; zle -N w; bindkey '^T' w")
			// Whatever is left of the line goes before the session's own
			// `exit` is typed after it. Registered after the session, so it
			// runs first.
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			// The line typed and its cursor moved back into the middle,
			// which is where a message written at the cursor does the most
			// harm: it covers the rest of the line.
			widgetKeys(t, control, "abcd\x02\x02")
			if err := screen.Await("abcd", widgetBudget); err != nil {
				t.Fatalf("the line was not drawn: %v", err)
			}
			mark := len(screen.Text())
			widgetKeys(t, control, "\x14")
			if err := screen.Await(row.says, widgetBudget); err != nil {
				t.Fatalf("no %q: %v", row.says, err)
			}
			if row.also != "" {
				if err := screen.Await(row.also, widgetBudget); err != nil {
					t.Fatalf("no %q: %v", row.also, err)
				}
			}
			after := screen.Text()[mark:]
			at := strings.Index(after, row.says)
			before := after[:at]
			if got := strings.HasSuffix(before, "\n"); got != row.fresh {
				t.Errorf("the message starts a row of its own: %v, want %v; written after %q",
					got, row.fresh, before)
			}
			if !row.fresh {
				return
			}
			// And the prompt is put back whole under it, upper row and all.
			if err := screen.Await(widgetMark, widgetBudget); err != nil {
				t.Fatalf("no prompt after the message: %v", err)
			}
			redrawn := screen.Text()[mark:]
			if !strings.Contains(redrawn[strings.Index(redrawn, row.says):], "HWROW") {
				t.Errorf("the prompt's upper row was not drawn again under the message:\n%s",
					smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
		})
	}
}

func widgetKeys(t *testing.T, control interface{ WriteString(string) (int, error) }, keys string) {
	t.Helper()
	if _, err := control.WriteString(keys); err != nil {
		t.Fatalf("typing %q: %v", keys, err)
	}
}
