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

// A right prompt set in a startup file is drawn against the right-hand edge,
// and a continuation line draws its own.
//
// Until #5893 neither was: only a theme ever filled the right half, and the
// prompt drawn from the parameters never read `RPS1`, `RPROMPT`, `RPS2` or
// `RPROMPT2`, so the screen held the left prompt and nothing else.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2, aarch64-apple-darwin25.4.0) at 40 columns with
// `PS1=$'up\n> '`: `RPS1=TIME` and `RPROMPT=TIME` each draw `TIME` ending one
// column short of the edge, it stays there while `ab` is typed and the cursor
// stays after the `ab`, `RPS2`/`RPROMPT2` is drawn the same way beside
// `dquote> `, and with `ZLE_RPROMPT_INDENT=0` it ends in the last column.
//
// Asserted on a terminal model: what matters is the *column* the right prompt
// is in and where the cursor is left, and neither is in the text.
func TestARightPromptFromTheParametersIsDrawn(t *testing.T) {
	const cols = 100 // the width jobNoticeSessionRC sizes the terminal to
	for _, c := range []struct {
		name, rc string
		// end is the column the right prompt's last cell is in.
		end int
	}{
		{"RPS1", "RPS1=TIME\nRPS2=MORE\n", cols - 2},
		{"RPROMPT", "RPROMPT=TIME\nRPROMPT2=MORE\n", cols - 2},
		{"indent 0", "RPS1=TIME\nRPS2=MORE\nZLE_RPROMPT_INDENT=0\n", cols - 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionRC(t, c.rc, "zsh", "-i")
			grid := func() *cellgrid.Grid {
				g := cellgrid.New(cols)
				_, _ = g.Write([]byte(screen.Text()))
				return g
			}
			// lastRow is the last row that begins with prefix, or -1.
			lastRow := func(g *cellgrid.Grid, prefix string) int {
				row := -1
				for r := range g.Rows() {
					if strings.HasPrefix(g.Text(r), prefix) {
						row = r
					}
				}
				return row
			}
			at := func(g *cellgrid.Grid, row int) string {
				var b strings.Builder
				for col := c.end - 3; col <= c.end; col++ {
					b.WriteString(g.Cell(row, col).Text)
				}
				return b.String()
			}
			await := func(what string, ok func(g *cellgrid.Grid) bool) *cellgrid.Grid {
				t.Helper()
				deadline := time.Now().Add(jobNoticeBudget)
				for g := grid(); ; g = grid() {
					if ok(g) {
						return g
					}
					if time.Now().After(deadline) {
						row, col := g.Cursor()
						t.Fatalf("%s; cursor at row %d, column %d:\n%s\nraw: %q",
							what, row, col, g, smoke.LastLines(screen.Text(), 6))
					}
					time.Sleep(10 * time.Millisecond)
				}
			}

			// At the first prompt, and still there with a line typed — the
			// line is drawn by a redraw rather than with the prompt, so the
			// two are separate routes to the same row.
			for _, typed := range []string{"", "ab"} {
				if typed != "" {
					if _, err := control.WriteString(typed); err != nil {
						t.Fatalf("typing %q: %v", typed, err)
					}
				}
				await("TIME was not drawn against the edge beside "+jobNoticeMark+typed, func(g *cellgrid.Grid) bool {
					p := lastRow(g, jobNoticeMark)
					row, col := g.Cursor()
					return p >= 0 && strings.TrimRight(g.Text(p), " ") == jobNoticeMark+typed+
						strings.Repeat(" ", c.end-3-len(jobNoticeMark+typed))+"TIME" &&
						at(g, p) == "TIME" && row == p && col == len(jobNoticeMark+typed)
				})
			}
			// Not a ladder: the upper row reaches the screen once.
			if g := grid(); strings.Count(g.String(), "JNROW") != 1 {
				t.Errorf("the upper row was drawn %d times:\n%s", strings.Count(g.String(), "JNROW"), g)
			}

			// A continuation line draws the second right prompt.
			if _, err := control.WriteString("\x15echo \"\r"); err != nil {
				t.Fatalf("typing: %v", err)
			}
			await("MORE was not drawn beside the continuation prompt", func(g *cellgrid.Grid) bool {
				p := lastRow(g, "dquote> ")
				row, col := g.Cursor()
				return p >= 0 && at(g, p) == "MORE" && row == p && col == len("dquote> ")
			})
			if _, err := control.WriteString("\"\r"); err != nil {
				t.Fatalf("typing: %v", err)
			}
		})
	}
}
