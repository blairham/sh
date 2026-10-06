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

// A completion listing too tall for the terminal is paged under LISTPROMPT
// while zsh/complist is loaded, rather than asked about (#6153).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 — see
// repl/listscroll.go for the table. Here the terminal is 24 rows by 100
// columns and 400 names list in 25 rows of 16, so a screenful is 23 rows and
// the prompt reports the last of them: the first screenful ends on row 23,
// whose last name is the 398th, and Return draws row 24, whose last is the
// 399th — 24 of 25 rows being 96%.
func TestALongListingIsPagedUnderListPrompt(t *testing.T) {
	const files = "mkdir lp && cd lp && for c in a b c d; do for i in {100..199}; do : >$c$i; done; done"
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	// bottom waits until the terminal's last row reads want.
	bottom := func(t *testing.T, screen *smoke.Screen, want string) *cellgrid.Grid {
		t.Helper()
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			r, _ := g.Cursor()
			if strings.TrimRight(g.Text(r), " ") == want {
				return g
			}
			if time.Now().After(deadline) {
				var rows []string
				for r := range g.Rows() {
					rows = append(rows, g.Text(r))
				}
				cr, cc := g.Cursor()
				t.Fatalf("never saw %q on the cursor's row (%d,%d):\n%s", want, cr, cc, strings.Join(rows, "\n"))
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

	t.Run("paged, a row at a time, then stopped by a key that is typed", func(t *testing.T) {
		control, screen := widgetSession(t, files, "zmodload zsh/complist", "LISTPROMPT='[%L][%M][%P]'")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "x \t")
		g := bottom(t, screen, "[23/25    ][398/400  ][Top   ]")
		// A screenful is the terminal's height less the prompt's row: 23
		// rows of the listing between the line and the prompt. (The grid
		// keeps every row it is given, so the line is still in it.)
		if r, _ := g.Cursor(); r-lastRowStarting(g, strings.TrimSpace(widgetMark))-1 != 23 {
			t.Errorf("drew %d rows before the prompt, want 23", r-lastRowStarting(g, strings.TrimSpace(widgetMark))-1)
		}
		if strings.Contains(screen.Text(), "do you wish") {
			t.Error("a paged listing was asked about first")
		}
		send(t, control, "\r")
		bottom(t, screen, "[24/25    ][399/400  ][96%   ]")
		send(t, control, "q")
		g = bottom(t, screen, widgetMark+"x q")
		// The listing stays where it stopped, its last row just above the
		// line, and nothing of the prompt is left.
		r, _ := g.Cursor()
		if got := strings.Fields(g.Text(r - 1)); len(got) == 0 || got[0] != "a123" {
			t.Errorf("the row above the line is %q, want the listing's 24th row", g.Text(r-1))
		}
		if strings.Contains(g.Text(r-1), "[") {
			t.Errorf("the prompt was left on the screen: %q", g.Text(r-1))
		}
	})

	t.Run("a screenful at a time to the end, and the line under it", func(t *testing.T) {
		control, screen := widgetSession(t, files, "zmodload zsh/complist", "LISTPROMPT='[%L][%M][%P]'")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "x \t")
		bottom(t, screen, "[23/25    ][398/400  ][Top   ]")
		send(t, control, "\t")
		g := bottom(t, screen, widgetMark+"x")
		r, _ := g.Cursor()
		if got := strings.Fields(g.Text(r - 1)); len(got) == 0 || got[0] != "a124" {
			t.Errorf("the row above the line is %q, want the listing's last", g.Text(r-1))
		}
	})

	// A listing that fits under the line is drawn whole, not paged, and the
	// cursor goes back up to the line: 368 names list in 23 rows, which
	// with the line's own row fill the 24. Measured on zsh 5.9.2 at 40 rows,
	// 39 rows under a one-row line come back to the line and 40 are paged;
	// here the listing's last row was ended, which scrolled the terminal,
	// and the line was drawn again under the listing.
	t.Run("a listing that exactly fits comes back to the line", func(t *testing.T) {
		control, screen := widgetSession(t,
			"mkdir lp && cd lp && for c in a b c d; do for i in {100..191}; do : >$c$i; done; done",
			"zmodload zsh/complist", "LISTPROMPT='[%L][%M][%P]'")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "x \t")
		mark := strings.TrimSpace(widgetMark)
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			p := lastRowStarting(g, mark)
			r, c := g.Cursor()
			if p >= 0 && strings.HasPrefix(g.Text(p+23), "a122") && r == p && c == len(widgetMark)+2 {
				if strings.Contains(screen.Text(), "[23/") {
					t.Error("a listing that fits was paged")
				}
				break
			}
			if time.Now().After(deadline) {
				var rows []string
				for r := range g.Rows() {
					rows = append(rows, g.Text(r))
				}
				t.Fatalf("the cursor did not come back to the line over the listing (cursor %d,%d):\n%s", r, c, strings.Join(rows, "\n"))
			}
			time.Sleep(20 * time.Millisecond)
		}
	})

	// The control: without the module, LISTPROMPT pages nothing and the
	// question is asked as it always was.
	t.Run("without zsh/complist the question is asked", func(t *testing.T) {
		control, screen := widgetSession(t, files, "LISTPROMPT='[%L][%M][%P]'")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "x \t")
		if err := screen.Await("do you wish to see all 400 possibilities", widgetBudget); err != nil {
			t.Fatalf("no question: %v\n%s", err, smoke.Readable(smoke.LastLines(screen.Text(), 4)))
		}
		send(t, control, "n")
	})
}
