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

// A control character a self-insert types goes in the line and is drawn as a
// caret in standout, on a real terminal and under a two-row prompt (#5972).
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, rendered
// cell by cell: `bindkey '^T' self-insert` and `ab ^T cd` with the cursor
// moved back twice is `ab^Tcd` on the prompt's row, the `^T` in two reverse
// cells and the cursor on the `c`; a widget running `zle .self-insert` on `^T`
// over `abcdefgh` with the cursor at 1 leaves `a^Tbcdefgh`, cursor 2; and a
// tab is the spaces to its stop on the screen's row. Here the first typed the
// character before it, the second typed nothing, and a tab could not be in
// the line at all.
func TestAControlCharacterIsTypedAndDrawnAsACaret(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	for _, row := range []struct {
		name, keys string
		// text is the prompt's row as it is shown, and caret the column a
		// `^` in standout starts at (-1 for none), and col where the cursor is.
		text       string
		caret, col int
	}{
		{"a key bound to self-insert types itself", "ab\x14cd\x02\x02", mark + " ab^Tcd", 6, 8},
		{"a widget's self-insert types the key", "\x19", mark + " a^Tbcdefgh", 5, 7},
		{"a tab is the spaces to its stop", "a\x0fZ", mark + " a   Z", -1, 9},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t,
				`bindkey '^T' self-insert`,
				`w() { BUFFER=abcdefgh; CURSOR=1; zle .self-insert -- }; zle -N w; bindkey '^Y' w`,
				`v() { LBUFFER+=$'\t' }; zle -N v; bindkey '^O' v`,
			)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			if row.keys == "\x19" {
				// The widget reads the key it was pressed with, which here
				// is `^Y`; the measured case is `^T`, so it is bound there
				// for this row.
				widgetType(t, control, screen, `bindkey '^T' w`)
				row.keys = "\x14"
			}
			if _, err := control.WriteString(row.keys); err != nil {
				t.Fatalf("typing: %v", err)
			}
			deadline := time.Now().Add(widgetBudget)
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				p := lastRowStarting(g, mark)
				r, c := g.Cursor()
				caretOK := row.caret < 0 || (p >= 0 && g.Cell(p, row.caret).Text == "^" &&
					g.Cell(p, row.caret).Reverse && g.Cell(p, row.caret+1).Reverse)
				if p >= 0 && g.Text(p) == row.text && caretOK && r == p && c == row.col {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("want the row %q, a caret at %d and the cursor at %d; got row %q, cursor %d,%d (prompt row %d)\n%s",
						row.text, row.caret, row.col, g.Text(max(p, 0)), r, c, p,
						smoke.Readable(smoke.LastLines(screen.Text(), 4)))
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

// lastRowStarting is the last row whose text begins with prefix, or -1.
func lastRowStarting(g *cellgrid.Grid, prefix string) int {
	row := -1
	for r := range g.Rows() {
		if strings.HasPrefix(g.Text(r), prefix) {
			row = r
		}
	}
	return row
}
