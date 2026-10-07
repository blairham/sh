// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/smoke"
)

// A yank is drawn in standout until the next key, as a paste is, and
// `zle_highlight`'s paste context decides it (#6271). Measured 2026-10-06
// through a pseudo-terminal against zsh 5.9.2, a two-row prompt: `echo ab`,
// `^W`, `^Y` draws `ab` reversed and the next key draws it plain; after
// `zle_highlight=(paste:none)` neither a yank nor a bracketed paste is marked.
// Here a yank was always plain and the paste always reversed.
func TestAYankIsDrawnAsPastedOnATerminal(t *testing.T) {
	control, screen := widgetSession(t)
	mark := strings.TrimSpace(widgetMark)
	col := len(widgetMark)
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
	}
	// await waits for the row and the cursor, and for each listed cell to be
	// reversed or not as asked.
	await := func(what, row string, cursor int, reversed map[int]bool) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := cellgrid.New(100)
			_, _ = g.Write([]byte(screen.Text()))
			p := lastRowStarting(g, mark)
			r, c := g.Cursor()
			ok := p >= 1 && g.Text(p-1) == "HWROW" && g.Text(p) == mark+" "+row && r == p && c == cursor
			for at, want := range reversed {
				ok = ok && g.Cell(p, at).Reverse == want
			}
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s: row %q, cursor %d,%d\n%s", what, g.Text(max(p, 0)), r, c,
					smoke.Readable(smoke.LastLines(screen.Text(), 4)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	fresh := func() {
		t.Helper()
		at := len(screen.Text())
		send("\x03")
		deadline := time.Now().Add(widgetBudget)
		for !strings.Contains(screen.Text()[at:], "HWROW") {
			if time.Now().After(deadline) {
				t.Fatalf("no fresh prompt:\n%s", smoke.Readable(screen.Text()[at:]))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	a, b := col+5, col+6

	send("echo ab")
	await("the line typed", "echo ab", col+7, nil)
	send("\x17")
	await("the word killed", "echo", col+5, nil)
	send("\x19")
	await("the yank reversed", "echo ab", col+7, map[int]bool{a: true, b: true})
	send("x")
	await("the yank drawn plainly after a key", "echo abx", col+8, map[int]bool{a: false, b: false})

	fresh()
	at := len(screen.Text())
	send("zle_highlight=(paste:none)\r")
	deadline := time.Now().Add(widgetBudget)
	for !strings.Contains(screen.Text()[at:], "HWROW") {
		if time.Now().After(deadline) {
			t.Fatalf("the assignment did not run:\n%s", smoke.Readable(screen.Text()[at:]))
		}
		time.Sleep(20 * time.Millisecond)
	}
	send("echo ab")
	await("the line typed", "echo ab", col+7, nil)
	send("\x17")
	await("the word killed", "echo", col+5, nil)
	send("\x19")
	await("the yank plain under paste:none", "echo ab", col+7, map[int]bool{a: false, b: false})
	fresh()
}
