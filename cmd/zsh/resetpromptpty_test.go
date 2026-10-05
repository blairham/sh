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
)

// `zle reset-prompt` renders the prompt again and draws it, with the line
// under it, from the row the editor believes the prompt starts on.
//
// #5940: it was not a widget here, so the call answered 1 and drew nothing,
// and a widget that printed and then reset the prompt left the line where
// the print had pushed it. Measured 2026-10-04 against zsh 5.9.2 through a
// pseudo-terminal with `PS1=$'up\n> '` and a widget that prints `hi`,
// assigns `PS1=$'UP2\n>> '` and runs `zle reset-prompt`, after typing `ab`:
//
//	\r\r\e[A…\e[JUP2\r\n>> ab
//
// so the upper row goes back over the row the print wrote on, the new prompt
// is drawn, and `$?` after the call is 0.
func TestResetPromptDrawsThePromptAgain(t *testing.T) {
	control, screen, home := jobNoticeSessionRC(t, `w() { print -r -- hi; PS1=$'RW2\nrw> '; zle reset-prompt; print -r -- "rc=$?" > $HOME/rc }
zle -N w; bindkey '^T' w
`, "zsh", "-i")
	// One write, `c` after the widget's key included, so all four keys are
	// read together: a key that arrives while the widget runs reaches a
	// terminal in its line discipline, which echoes it where the cursor is.
	if _, err := control.WriteString("ab\x14c"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	// The new prompt drawn over the row the print wrote on, and the key
	// after the widget drawn under it rather than under the old one: the
	// read carries the prompt it began with, and every redraw after a
	// reset-prompt has to ask for the one on the screen.
	drawn := func(g *cellgrid.Grid) bool {
		row, col := g.Cursor()
		return g.Rows() >= 3 && g.Text(0) == "JNROW" && g.Text(1) == "RW2" && g.Text(2) == "rw> abc" &&
			row == 2 && col == len("rw> abc")
	}
	for deadline := time.Now().Add(jobNoticeBudget); ; time.Sleep(10 * time.Millisecond) {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		if drawn(g) {
			break
		}
		if time.Now().After(deadline) {
			row, col := g.Cursor()
			t.Fatalf("the prompt was not drawn again over the print; cursor at row %d, column %d:\n%s\nraw: %q",
				row, col, g, screen.Text())
		}
	}
	rc := func() string {
		data, _ := os.ReadFile(filepath.Join(home, "rc"))
		return strings.TrimSpace(string(data))
	}
	for deadline := time.Now().Add(jobNoticeBudget); rc() != "rc=0"; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("zle reset-prompt answered %q, want rc=0", rc())
		}
	}
}
