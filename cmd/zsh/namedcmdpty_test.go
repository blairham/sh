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

// which-command, run-help and execute-named-cmd on the keys zsh's emacs keymap
// has them on, drawn as they go (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt, `echo abc def` with the cursor at 10: `M-?` draws
// `which-command echo` over the line, prints `echo`, and the next prompt holds
// `echo abc def` with the cursor at 10 again; `M-h` is the same through
// `run-help`. `M-x` draws `execute: _` on the row under the line, `up-ca` and
// Tab fill it to `execute: up-case-word_`, `up-` and Tab list the five names
// beginning that way, and Return runs the widget and takes the row away. Here
// all three keys did nothing.
func TestTheCommandKeysAreDrawn(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	// until waits for the screen to satisfy ok, given the grid and the last
	// prompt row.
	until := func(t *testing.T, screen *smoke.Screen, what string, ok func(g *cellgrid.Grid, p int) bool) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			if p := lastRowStarting(g, mark); p >= 1 && g.Text(p-1) == "HWROW" && ok(g, p) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s:\n%s", what, smoke.Readable(smoke.LastLines(screen.Text(), 8)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	at := func(g *cellgrid.Grid, p int, text string, col int) bool {
		r, c := g.Cursor()
		return g.Text(p) == mark+" "+text && r == p && c == len(widgetMark)+col
	}
	send := func(t *testing.T, control interface{ WriteString(string) (int, error) }, keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
	}
	start := func(t *testing.T, rc ...string) (interface{ WriteString(string) (int, error) }, *smoke.Screen) {
		control, screen := widgetSession(t, rc...)
		t.Cleanup(func() { _, _ = control.WriteString("\x07\x01\x0b") })
		send(t, control, "echo abc def\x02\x02")
		until(t, screen, "the line typed", func(g *cellgrid.Grid, p int) bool { return at(g, p, "echo abc def", 10) })
		return control, screen
	}

	for _, row := range []struct{ name, keys, word string }{
		{"M-?", "\x1b?", "which-command"},
		{"M-h", "\x1bh", "run-help"},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := start(t, "alias "+row.word+"='print -r -- ASKED'")
			send(t, control, row.keys)
			until(t, screen, "the question run and the line back", func(g *cellgrid.Grid, p int) bool {
				return at(g, p, "echo abc def", 10) && p >= 4 && g.Text(p-2) == "ASKED echo" &&
					g.Text(p-3) == mark+" "+row.word+" echo"
			})
		})
	}

	t.Run("M-x", func(t *testing.T) {
		control, screen := start(t)
		send(t, control, "\x1bx")
		until(t, screen, "the minibuffer under the line", func(g *cellgrid.Grid, p int) bool {
			return g.Text(p) == mark+" echo abc def" && g.Text(p+1) == "execute: _"
		})
		send(t, control, "up-ca\t")
		until(t, screen, "the name completed", func(g *cellgrid.Grid, p int) bool {
			return g.Text(p) == mark+" echo abc def" && g.Text(p+1) == "execute: up-case-word_"
		})
		send(t, control, "\x15up-\t")
		until(t, screen, "the names listed under the row", func(g *cellgrid.Grid, p int) bool {
			return g.Text(p) == mark+" echo abc def" && g.Text(p+1) == "execute: up-_" &&
				strings.HasPrefix(g.Text(p+2), "up-case-word") && strings.Contains(g.Text(p+2), "up-line-or-history")
		})
		send(t, control, "\x15up-case-word\r")
		until(t, screen, "the widget run and the row gone", func(g *cellgrid.Grid, p int) bool {
			return at(g, p, "echo abc dEF", 12) && g.Text(p+1) == ""
		})
	})
}
