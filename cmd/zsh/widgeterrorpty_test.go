// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/smoke"
)

// A fatal error in a widget costs the line and nothing else, on a real
// terminal (#5959).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with this startup file: typing `ran=line`, pressing the key, then ^Xb draws
// `CHECK s=1 ran=none END` for each of the three keys — the diagnostic, the
// line given up without running, `$?` 1, and the editor still reading keys
// and running widgets. Before the fix this shell left the error standing:
// every later command was skipped, so ^Xb's widget never ran, and the
// terminal was not taken back for the line.
func TestAFatalErrorInAWidgetCostsTheLine(t *testing.T) {
	for _, row := range []struct{ name, key, says string }{
		{"an expansion that stops", "\x14", "nosuch: gone"},
		{"an assignment to a read-only parameter", "\x18k", "read-only variable: KEYS"},
		{"the error in a widget another widget called", "\x18n", "nosuch: gone"},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t, `ran=none
d() { : ${nosuch?gone}; ran=widget }; zle -N d; bindkey '^T' d
k() { KEYS=x; ran=widget }; zle -N k; bindkey '^Xk' k
n() { zle d; ran=caller }; zle -N n; bindkey '^Xn' n
b() { BUFFER="CHECK s=$? ran=$ran END" }; zle -N b; bindkey '^Xb' b
c() { BUFFER= }; zle -N c; bindkey '^Xc' c
`)
			if _, err := control.WriteString("ran=line" + row.key); err != nil {
				t.Fatalf("typing: %v", err)
			}
			if err := screen.Await(row.says, widgetBudget); err != nil {
				t.Fatalf("no diagnostic %q: %v\n%q", row.says, err, smoke.LastLines(screen.Text(), 6))
			}
			// The line drawn once more under the message, and then the
			// fresh prompt: two marks, and the keys go only after the second,
			// when the editor is reading the next line.
			for range 2 {
				if err := screen.Await(widgetMark, widgetBudget); err != nil {
					t.Fatalf("no fresh prompt after the error: %v\n%q", err, smoke.LastLines(screen.Text(), 6))
				}
			}
			if _, err := control.WriteString("\x18b"); err != nil {
				t.Fatalf("pressing ^Xb: %v", err)
			}
			const want = "CHECK s=1 ran=none END"
			if err := screen.Await(want, widgetBudget); err != nil {
				t.Fatalf("after the error the session is not\n%s\n%v\n%q", want, err, smoke.LastLines(screen.Text(), 6))
			}
			if _, err := control.WriteString("\x18c"); err != nil {
				t.Fatalf("clearing: %v", err)
			}
		})
	}
}
