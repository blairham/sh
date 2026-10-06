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

// push-line, quote-line, accept-and-hold and `^X u` on the keys zsh's emacs
// keymap has them on, drawn as they change the line (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt, `echo abc def` with the cursor on the `c`: `M-q` empties
// the line and it comes back after the next command with the cursor on the
// `c`; `M-'` draws `'echo abc def'` with the cursor at its end; `M-a` prints
// `abc def` and the next prompt holds the line, cursor on the `c`; `^X u`
// takes the `f` off. Here none of them drew anything, and Return ran `echo
// abc def` unchanged.
func TestTheLineKeysAreDrawn(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	// until waits for the last prompt row to read want with the cursor at
	// col on it, the prompt's upper row above it.
	//
	// ran, when it is not empty, is a row of output that has to be on the
	// screen above that prompt: what a line that ran printed.
	until := func(t *testing.T, screen *smoke.Screen, want string, col int, ran string) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			p := lastRowStarting(g, mark)
			r, c := g.Cursor()
			if p >= 1 && g.Text(p) == strings.TrimRight(mark+" "+want, " ") && r == p &&
				c == len(widgetMark)+col && g.Text(p-1) == "HWROW" && (ran == "" || p >= 2 && g.Text(p-2) == ran) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("want the row %q under HWROW and the cursor at %d; got row %q, cursor %d,%d (prompt row %d)\n%s",
					mark+" "+want, len(widgetMark)+col, g.Text(max(p, 0)), r, c, p,
					smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	type step struct {
		keys, want string
		col        int
		ran        string
	}
	for _, row := range []struct {
		name  string
		steps []step
	}{
		{"M-q", []step{
			{"\x1bq", "", 0, ""},
			{"echo Z$((1+1))\r", "echo abc def", 7, "Z2"},
		}},
		{"M-'", []step{{"\x1b'", "'echo abc def'", 14, ""}}},
		{"M-a", []step{{"\x1ba", "echo abc def", 7, "abc def"}}},
		{"^X u", []step{{"\x18u", "echo abc de", 11, ""}}},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			if _, err := control.WriteString("echo abc def" + strings.Repeat("\x02", 5)); err != nil {
				t.Fatal(err)
			}
			until(t, screen, "echo abc def", 7, "")
			for _, s := range row.steps {
				if _, err := control.WriteString(s.keys); err != nil {
					t.Fatal(err)
				}
				until(t, screen, s.want, s.col, s.ran)
			}
		})
	}
}
