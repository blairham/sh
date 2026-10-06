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

// `^V` in vi insert mode is vi-quoted-insert: a plain caret is put in the row
// at the cursor while it waits, and the key that follows takes its place
// (#6251).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a two-row
// prompt, `bindkey -v`, `abc` with the cursor on the `b`: `^V` draws `a^bc`
// with the cursor on the caret, and `^A` then draws `a^Abc` with the `^A` in
// two reverse cells and the cursor on the `b`. `^V ^C` rings and abandons the
// line. Here `^V` did nothing at all and `^A` was ignored.
func TestViQuotedInsertDrawsItsPlaceholder(t *testing.T) {
	control, screen := widgetSession(t, "bindkey -v")
	mark := strings.TrimSpace(widgetMark)
	col := len(widgetMark)
	await := func(what string, ok func(g *cellgrid.Grid, p, r, c int) bool) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := cellgrid.New(100)
			_, _ = g.Write([]byte(screen.Text()))
			p := lastRowStarting(g, mark)
			r, c := g.Cursor()
			if p >= 1 && g.Text(p-1) == "HWROW" && r == p && ok(g, p, r, c) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s: row %q, cursor %d,%d (prompt row %d)\n%s",
					what, g.Text(max(p, 0)), r, c, p, smoke.Readable(smoke.LastLines(screen.Text(), 4)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	send := func(keys string) {
		t.Helper()
		if _, err := control.WriteString(keys); err != nil {
			t.Fatal(err)
		}
	}

	send("abc\x1b[D\x1b[D")
	await("the line typed", func(g *cellgrid.Grid, p, _, c int) bool {
		return g.Text(p) == mark+" abc" && c == col+1
	})
	send("\x16")
	await("the caret drawn at the cursor", func(g *cellgrid.Grid, p, _, c int) bool {
		return g.Text(p) == mark+" a^bc" && c == col+1 && !g.Cell(p, col+1).Reverse
	})
	send("\x01")
	await("the ^A in its place", func(g *cellgrid.Grid, p, _, c int) bool {
		return g.Text(p) == mark+" a^Abc" && c == col+3 &&
			g.Cell(p, col+1).Reverse && g.Cell(p, col+2).Reverse
	})

	// And `^V ^C` rings, takes the caret away and gives the line up.
	before := len(screen.Text())
	send("\x16")
	await("the caret drawn again", func(g *cellgrid.Grid, p, _, c int) bool {
		return g.Text(p) == mark+" a^A^bc" && c == col+3
	})
	send("\x03")
	await("a fresh prompt", func(g *cellgrid.Grid, p, _, c int) bool {
		return g.Text(p) == mark && c == col
	})
	if after := screen.Text()[before:]; !strings.Contains(after, "\a") {
		t.Errorf("^V ^C did not ring:\n%s", smoke.Readable(after))
	}
}
