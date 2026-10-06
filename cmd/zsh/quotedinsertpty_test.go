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

// `^V` puts the next key in the line as it is, drawn as a caret in standout
// where it is a control character (#6241).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `zsh -f`,
// a two-row prompt, `echo abc def` with the cursor on the `c`: `^V ^A` draws
// `echo ab^Ac def` with the `^A` in two reverse cells and the cursor on the
// `c`, and `^V` and Return puts a `^M` there rather than running the line.
// Here `^V` was ignored, so `^A` went to the start of the line and Return ran
// it.
func TestQuotedInsertIsTypedAndDrawn(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	for _, row := range []struct {
		name, keys, text string
		caret, col       int
	}{
		{"^V ^A", "\x16\x01", mark + " echo ab^Ac def", 11, 13},
		{"^V Return", "\x16\r", mark + " echo ab^Mc def", 11, 13},
		{"^V and a letter", "\x16x", mark + " echo abxc def", -1, 12},
		{"a count", "\x1b2\x16\x01", mark + " echo ab^A^Ac def", 11, 15},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			if _, err := control.WriteString("echo abc def" + strings.Repeat("\x02", 5)); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(widgetBudget)
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
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
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				p := lastRowStarting(g, mark)
				r, c := g.Cursor()
				caretOK := row.caret < 0 || (p >= 0 && g.Cell(p, row.caret).Text == "^" &&
					g.Cell(p, row.caret).Reverse && g.Cell(p, row.caret+1).Reverse)
				if p >= 1 && g.Text(p) == row.text && g.Text(p-1) == "HWROW" && caretOK && r == p && c == row.col {
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
