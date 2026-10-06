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

// The case keys and transpose-words, on the keys zsh's emacs keymap has them
// on, drawn as they change the line (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt, `echo abc def` with the cursor on the `c`: `M-u` draws
// `C` over it and the cursor goes past it, `M-l` and `M-c` the same, and `M-t`
// draws the line again as `abc echo def` with the cursor after `echo`. Here
// none of the four drew anything, and Return ran `echo abc def` unchanged.
func TestTheCaseKeysAndTransposeWordsAreDrawn(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	for _, row := range []struct {
		name, keys, line string
		col              int
	}{
		{"M-u", "\x1bu", "echo abC def", 8},
		{"M-U", "\x1bU", "echo abC def", 8},
		{"M-c", "\x1bc", "echo abC def", 8},
		{"M-l on a capital", "\x1bb\x1bu\x02\x02\x02\x1bl", "echo abc def", 8},
		{"M-t", "\x1bt", "abc echo def", 8},
		{"ESC 2 M-u", "\x1bb\x1b2\x1bu", "echo ABC DEF", 12},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			if _, err := control.WriteString("echo abc def" + strings.Repeat("\x02", 5)); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(widgetBudget)
			for {
				g := render(screen)
				_, c := g.Cursor()
				if p := lastRowStarting(g, mark); p >= 0 && g.Text(p) == mark+" echo abc def" && c == len(widgetMark)+7 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("never saw the line typed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 4)))
				}
				time.Sleep(20 * time.Millisecond)
			}
			if _, err := control.WriteString(row.keys); err != nil {
				t.Fatal(err)
			}
			want := mark + " " + row.line
			for {
				g := render(screen)
				p := lastRowStarting(g, mark)
				r, c := g.Cursor()
				if p >= 1 && g.Text(p) == want && r == p && c == len(widgetMark)+row.col && g.Text(p-1) == "HWROW" {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("want the row %q under HWROW and the cursor at %d; got row %q, cursor %d,%d (prompt row %d)\n%s",
						want, len(widgetMark)+row.col, g.Text(max(p, 0)), r, c, p,
						smoke.Readable(smoke.LastLines(screen.Text(), 4)))
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
