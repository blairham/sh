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

// Text typed in the same write as a key that draws nothing is on the screen
// while the shell waits, under a two-row prompt.
//
// #5881: the draw a typed character puts off while more input is in hand was
// flushed only by a later character, so `abc` and an empty paste in one write
// left the prompt bare until the next keystroke. Measured 2026-10-04 through
// a pseudo-terminal against /opt/homebrew/bin/zsh (zsh 5.9.2) with
// `PS1=$'up\n> '`: the row reads `> abc` with the cursor after it.
//
// The write can reach the shell split in two, which passes against the bug
// as well; the deterministic half is
// TestTextTypedBeforeAKeyThatDrawsNothingIsDrawnBeforeTheWait in repl.
func TestTextBeforeAnEmptyPasteIsDrawnAtOnce(t *testing.T) {
	control, screen, _ := jobNoticeSessionRC(t, "", "zsh", "-i")
	if _, err := control.WriteString("abc\x1b[200~\x1b[201~"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		p := -1
		for r := range g.Rows() {
			if strings.HasPrefix(g.Text(r), jobNoticeMark) {
				p = r
			}
		}
		row, col := g.Cursor()
		if p >= 0 && g.Text(p) == jobNoticeMark+"abc" && row == p && col == len(jobNoticeMark+"abc") {
			if n := strings.Count(g.String(), "JNROW"); n != 1 {
				t.Errorf("the upper row was drawn %d times:\n%s", n, g)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("abc was not drawn while the shell waited; cursor at row %d, column %d:\n%s\nraw: %q",
				row, col, g, smoke.LastLines(screen.Text(), 4))
		}
		time.Sleep(10 * time.Millisecond)
	}
}
