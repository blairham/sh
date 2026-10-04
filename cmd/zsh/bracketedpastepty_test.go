// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/smoke"
)

// A paste the terminal marked is text in the line, newlines and all, and
// nothing in it runs until Return is pressed.
//
// Until #5865 this was true of the bash dialect and false of this one. zsh's
// standard keymap puts sequences beginning `\e[` in the binding table from the
// first prompt, and the lookup that reads that table gave up on the paste's
// opening marker at `\e[2`: it dropped those three bytes, `00~` and the paste
// behind it were typed a key at a time, and the first newline ran the first
// line. Measured at v0.0.25 with this paste: `> 00~echo one`, then `zsh:
// command not found: 00~echo`, then `> echo two01~` left in the line.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2, aarch64-apple-darwin25.4.0) with `PS1=$'upper row\n> '` and a
// paste of `echo one⏎echo two`: both lines drawn in reverse video under the
// prompt, the cursor after `two`, nothing run; Return runs both.
//
// Asserted on a terminal model and not on the text the shell wrote: the bug's
// screen holds `echo one` and `echo two` too, and only where they are, and
// where the cursor is, tells the two apart. What ran is read off the files the
// two lines create, which nothing on the screen can fake.
func TestABracketedPasteIsTextInTheLineUntilReturn(t *testing.T) {
	control, screen, home := jobNoticeSessionRC(t, "", "zsh", "-i")
	const (
		first  = ": > ran1"
		second = ": > ran2"
	)
	if _, err := control.WriteString("\x1b[200~" + first + "\n" + second + "\x1b[201~"); err != nil {
		t.Fatalf("pasting: %v", err)
	}

	// The paste drawn: the prompt's own row holding the first line, the row
	// under it the second, and the cursor at the end of the second.
	grid := func() *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	promptRow := func(g *cellgrid.Grid) int {
		row := -1
		for r := range g.Rows() {
			if strings.HasPrefix(g.Text(r), jobNoticeMark) {
				row = r
			}
		}
		return row
	}
	drawn := func(g *cellgrid.Grid) bool {
		p := promptRow(g)
		row, col := g.Cursor()
		return p >= 0 && g.Text(p) == jobNoticeMark+first && g.Text(p+1) == second &&
			row == p+1 && col == len(second)
	}
	deadline := time.Now().Add(jobNoticeBudget)
	for g := grid(); !drawn(g); g = grid() {
		if time.Now().After(deadline) {
			row, col := g.Cursor()
			t.Fatalf("the paste was not drawn as two lines in the buffer; cursor at row %d, column %d:\n%s\nraw: %q",
				row, col, g, screen.Text())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// And drawn as a paste, which is how a person can tell it has not run:
	// in reverse video, the way both shells that bracket a paste mark one.
	g := grid()
	p := promptRow(g)
	for _, at := range [][2]int{{p, len(jobNoticeMark)}, {p + 1, 0}, {p + 1, len(second) - 1}} {
		if c := g.Cell(at[0], at[1]); !c.Reverse {
			t.Errorf("the pasted %q at row %d, column %d is not marked as pasted:\n%s\nraw: %q",
				c.Text, at[0], at[1], g, screen.Text())
		}
	}

	// Nothing ran. A quiet period rather than one look: the bug ran the first
	// line within milliseconds of the paste, so a file that is coming is here
	// well inside it.
	ran := func(name string) bool {
		_, err := os.Stat(filepath.Join(home, name))
		return err == nil
	}
	for quiet := time.Now().Add(500 * time.Millisecond); time.Now().Before(quiet); time.Sleep(10 * time.Millisecond) {
		if ran("ran1") || ran("ran2") {
			t.Fatalf("part of the paste ran before Return was pressed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
	}

	// Return runs the text, both lines of it — which is what makes the
	// silence above a measurement and not a session that runs nothing.
	if _, err := control.WriteString("\r"); err != nil {
		t.Fatalf("pressing Return: %v", err)
	}
	for deadline := time.Now().Add(jobNoticeBudget); !ran("ran1") || !ran("ran2"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("Return did not run both pasted lines (ran1 %v, ran2 %v):\n%s",
				ran("ran1"), ran("ran2"), smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
	}
}
