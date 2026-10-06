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

// `^D` on a line with something typed is delete-char-or-list, and both halves
// are drawn (#6233).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 under a
// two-row prompt: `echo abc`, Left and `^D` takes the `c` off the screen at
// once, and Return runs `echo ab`; `ls x` and `^D` in a directory holding xa,
// xb and xc lists `xa  xb  xc` under the line and puts the cursor back at the
// end of it, the upper row of the prompt untouched — with and without
// compinit, which redefines the widget. Here the first deleted the character
// and drew nothing, so `echo abc` stayed on the screen, and the second drew
// nothing at all.
func TestControlDOnATypedLineDeletesOrLists(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	until := func(t *testing.T, screen *smoke.Screen, what string, ok func(g *cellgrid.Grid, p int) bool) {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			if p := lastRowStarting(g, mark); p >= 0 && ok(g, p) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("never saw %s:\n%s", what, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	send := func(t *testing.T, c interface{ WriteString(string) (int, error) }, keys string) {
		t.Helper()
		if _, err := c.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}
	files := ": >xa; : >xb; : >xc"

	t.Run("under the cursor it deletes, and draws the deletion", func(t *testing.T) {
		control, screen := widgetSession(t)
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "echo abc\x02")
		until(t, screen, "the line typed and the cursor on the c", func(g *cellgrid.Grid, p int) bool {
			r, c := g.Cursor()
			return g.Text(p) == mark+" echo abc" && r == p && c == len(widgetMark)+7
		})
		send(t, control, "\x04")
		until(t, screen, "the c taken off the screen", func(g *cellgrid.Grid, p int) bool {
			r, c := g.Cursor()
			return g.Text(p) == mark+" echo ab" && r == p && c == len(widgetMark)+7
		})
	})

	for _, row := range []struct{ name, rc string }{
		{"at the end of the line it lists", files},
		{"and lists with the widget redefined as compinit leaves it", files +
			"\nzle -C delete-char-or-list .delete-char-or-list _main_complete"},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t, row.rc)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			send(t, control, "ls x")
			until(t, screen, "the line typed", func(g *cellgrid.Grid, p int) bool {
				return g.Text(p) == mark+" ls x"
			})
			send(t, control, "\x04")
			until(t, screen, "the matches listed under the line, and the cursor back on it", func(g *cellgrid.Grid, p int) bool {
				r, c := g.Cursor()
				return r == p && c == len(widgetMark)+4 && g.Text(p) == mark+" ls x" &&
					strings.TrimSpace(g.Text(p+1)) == "xa  xb  xc" && g.Text(p-1) == "HWROW"
			})
		})
	}

	// And `vared` without -e: the key on an emptied line is the same action,
	// and lists rather than ending the read. Measured against zsh 5.9.2, the
	// line emptied and `^D` asks `do you wish to see all 1064 possibilities`.
	t.Run("in vared, an emptied line lists", func(t *testing.T) {
		control, screen := widgetSession(t, "LISTMAX=3")
		t.Cleanup(func() { _, _ = control.WriteString("\x03\x01\x0b") })
		send(t, control, "v=xyz; vared v\n")
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			r, c := g.Cursor()
			if g.Text(r) == "xyz" && c == 3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("vared never drew its line:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 6)))
			}
			time.Sleep(20 * time.Millisecond)
		}
		send(t, control, "\x15\x04")
		if err := screen.Await("do you wish to see all", widgetBudget); err != nil {
			t.Fatalf("^D on the emptied line did not offer to list: %v\n%s", err,
				smoke.Readable(smoke.LastLines(screen.Text(), 6)))
		}
		send(t, control, "n")
	})
}
