// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// `zle -M` puts a message on the row under the line and it stays there while
// the line is edited, on a real terminal (#5942).
//
// Measured 2026-10-04 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal:
// `zle -M "hello msg"` is status 0 and writes the message on the row under the
// line at once; the keys typed after it leave it there; a second `zle -M`
// replaces it, `zle -M ""` takes it away, and when the line is accepted the
// row is cleared before the command's output reaches it. Before the fix this
// shell answered `-M is not implemented yet` at status 1 and drew nothing.
//
// Read through a cell grid, because what is asserted is which row holds what
// once every redraw has landed — not which bytes went out.
func TestAMessageStaysUnderTheLine(t *testing.T) {
	control, screen := widgetSession(t, `m() { zle -M "MSG$((6*7))" }; zle -N m; bindkey '^Xm' m
e() { zle -M "" }; zle -N e; bindkey '^Xe' e
q() { BUFFER="DONE$BUFFER" }; zle -N q; bindkey '^Xq' q
`)
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	await := func(want string) {
		t.Helper()
		if err := screen.Await(want, widgetBudget); err != nil {
			t.Fatalf("want %q: %v\n%q", want, err, smoke.LastLines(screen.Text(), 6))
		}
	}
	// awaitRow waits for a row of the screen to read text, for a line drawn
	// by a repaint that resumes part-way along it — whose bytes never hold
	// the whole line in one piece.
	awaitRow := func(text string) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for time.Now().Before(deadline) {
			g := grid(screen)
			for r := range g.Rows() {
				if strings.HasSuffix(g.Text(r), text) {
					return
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatalf("no row reads %q:\n%s", text, grid(screen).String())
	}
	// under is the text of the row after the last one holding line.
	under := func(line string) string {
		t.Helper()
		g := grid(screen)
		for r := g.Rows() - 1; r >= 0; r-- {
			if strings.Contains(g.Text(r), line) {
				return g.Text(r + 1)
			}
		}
		t.Fatalf("no row holds %q:\n%s", line, g.String())
		return ""
	}

	send("ab\x18m")
	await("MSG42")
	// A key after the message, and a widget after that to say it is done.
	send("c\x18q")
	await("DONEabc")
	// Polled, because the message goes back under the line *after* the
	// line is drawn — see editor.redraw — so a single read can land between
	// the two writes (#6094).
	got := under("DONEabc")
	for deadline := time.Now().Add(widgetBudget); got != "MSG42" && time.Now().Before(deadline); got = under("DONEabc") {
		time.Sleep(5 * time.Millisecond)
	}
	if got != "MSG42" {
		t.Errorf("under the line after more keys: %q, want %q", got, "MSG42")
	}
	send("\x18e\x18q")
	awaitRow("DONEDONEabc")
	if got := under("DONEDONEabc"); got != "" {
		t.Errorf("under the line after zle -M \"\": %q, want nothing", got)
	}
	// And a message still showing when the line ends is cleared before the
	// command's output lands on its row.
	send("\x18m")
	await("MSG42")
	send("\x15echo OUT$((1+1))\r")
	await("OUT2\r\n")
	if got := under("echo OUT$((1+1))"); got != "OUT2" {
		t.Errorf("under the accepted line: %q, want the output alone", got)
	}
}
