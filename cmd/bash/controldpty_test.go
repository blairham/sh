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

// `^D` on a line with something typed is delete-char, and the deletion is
// drawn (#6233).
//
// Measured 2026-10-06 through a pseudo-terminal against bash 5.3.20 under a
// two-row prompt: `echo abc`, Left and `^D` writes `\b\e[K` at once, taking
// the `c` off the screen, and Return runs `echo ab`. At the end of the line it
// rings the bell and lists nothing — the other dialect's key lists there. Here
// the deletion was made and nothing drawn, so `echo abc` stayed on the screen.
func TestControlDOnATypedLineDrawsTheDeletion(t *testing.T) {
	control, screen := interruptSession(t, ": >xa; : >xb\n")
	t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
	until := func(what string, ok func(g *cellgrid.Grid, p int) bool) {
		t.Helper()
		deadline := time.Now().Add(interruptBudget)
		for {
			g := cellgrid.New(100)
			_, _ = g.Write([]byte(screen.Text()))
			p := -1
			for r := range g.Rows() {
				if strings.HasPrefix(g.Text(r), interruptMark) {
					p = r
				}
			}
			if p >= 0 && ok(g, p) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s:\n%s", what, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	if _, err := control.WriteString("echo abc\x02"); err != nil {
		t.Fatal(err)
	}
	until("the line typed and the cursor on the c", func(g *cellgrid.Grid, p int) bool {
		r, c := g.Cursor()
		return g.Text(p) == interruptMark+"echo abc" && r == p && c == len(interruptMark)+7
	})
	if _, err := control.WriteString("\x04"); err != nil {
		t.Fatal(err)
	}
	until("the c taken off the screen", func(g *cellgrid.Grid, p int) bool {
		r, c := g.Cursor()
		return g.Text(p) == interruptMark+"echo ab" && r == p && c == len(interruptMark)+7
	})

	// And at the end of a word with matches, nothing is listed: the key
	// deletes, and there is nothing to delete.
	if _, err := control.WriteString("\x01\x0bls x\x04z"); err != nil {
		t.Fatal(err)
	}
	until("the line after the key", func(g *cellgrid.Grid, p int) bool {
		return g.Text(p) == interruptMark+"ls xz"
	})
	if strings.Contains(screen.Text(), "xa") {
		t.Errorf("^D at the end of the line listed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 6)))
	}
}
