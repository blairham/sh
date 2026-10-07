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

// zsh's vi insert keymap on a real terminal (#6272). Measured 2026-10-06
// through a pseudo-terminal against zsh 5.9.2, a two-row prompt and `bindkey
// -v`: `ab`, `^B`, `^T` draws `ab^B^T` with each control character a caret in
// standout; an arrow still moves; and `aa`, then ESC, `A` and `b` in one write,
// then Backspace twice, deletes the `b` and rings, because `A` began a new
// stretch of insert mode. Here `^B` and `^T` moved and transposed, and the
// Backspaces deleted both characters.
func TestZshViInsertKeymapOnATerminal(t *testing.T) {
	control, screen := widgetSession(t, "bindkey -v")
	mark := strings.TrimSpace(widgetMark)
	col := len(widgetMark)
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
	}
	await := func(what, row string, cursor int) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := cellgrid.New(100)
			_, _ = g.Write([]byte(screen.Text()))
			p := lastRowStarting(g, mark)
			r, c := g.Cursor()
			if p >= 1 && g.Text(p-1) == "HWROW" && g.Text(p) == mark+" "+row && r == p && c == cursor {
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

	send("ab")
	await("the line typed", "ab", col+2)
	send("\x02\x14")
	await("^B and ^T typed", "ab^B^T", col+6)

	fresh()
	send("abc")
	await("the line typed", "abc", col+3)
	send("\x1b[DX")
	await("the arrow move and the X", "abXc", col+3)

	fresh()
	send("aa")
	await("the line typed", "aa", col+2)
	send("\x1bAb")
	await("the b appended", "aab", col+3)
	before := len(screen.Text())
	send("\x7f")
	await("the b deleted", "aa", col+2)
	send("\x7f")
	deadline := time.Now().Add(widgetBudget)
	for !strings.Contains(screen.Text()[before:], "\a") {
		if time.Now().After(deadline) {
			t.Fatalf("no bell for Backspace past the insert:\n%s", smoke.Readable(screen.Text()[before:]))
		}
		time.Sleep(20 * time.Millisecond)
	}
	await("the line left alone", "aa", col+2)
	fresh()
}
