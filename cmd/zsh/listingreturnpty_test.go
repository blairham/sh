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

// After a listing the cursor goes back up to the line, and the listing stays
// under it while the line is edited (#6129) — ALWAYS_LAST_PROMPT, on by
// default.
//
// Measured 2026-10-05 against zsh 5.9.2 under `zsh -i` on a pseudo-terminal
// with a two-row prompt, rendered cell by cell: `ls x` and Tab over three
// matches leaves `P> ls x` on the prompt's row with the cursor at its end and
// `xa  xb  xc` on the row under it; `b`, two Backspaces and `yy` edit that row
// and leave the listing; `unsetopt alwayslastprompt` draws the line again
// under the listing instead. Here the line was always drawn again under it.
//
// And drawn again, it is drawn whole: measured 2026-10-06, zsh writes the
// ground and every row of the prompt under the listing, where this drew the
// row the line is on alone and left the upper row above the listing (#6209).
func TestAListingReturnsTheCursorToTheLine(t *testing.T) {
	const rc = `f() { compadd alpha1 alpha2 alpha3 }
zle -C cw complete-word f; bindkey '^T' cw
`
	for _, tc := range []struct {
		name, setup string
		back        bool
	}{
		{"on, by default", "", true},
		{"off", "unsetopt alwayslastprompt\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			control, screen := widgetSession(t, tc.setup+rc)
			if _, err := control.WriteString("a\x14\x14"); err != nil {
				t.Fatal(err)
			}
			if err := screen.Await("alpha3", widgetBudget); err != nil {
				t.Fatalf("no listing: %v\n%s", err, smoke.Readable(smoke.LastLines(screen.Text(), 4)))
			}
			if _, err := control.WriteString("b"); err != nil {
				t.Fatal(err)
			}
			deadline := time.Now().Add(widgetBudget)
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				p := lastRowStarting(g, widgetMark)
				listing := -1
				for r := range g.Rows() {
					if strings.HasPrefix(g.Text(r), "alpha1") {
						listing = r
					}
				}
				r, c := g.Cursor()
				ok := p >= 0 && g.Text(p) == widgetMark+"alphab" && r == p && c == len(widgetMark+"alphab")
				if tc.back {
					ok = ok && listing == p+1
				} else {
					// The whole prompt under the listing, its upper row
					// included (#6209).
					ok = ok && listing == p-2 && g.Text(p-1) == "HWROW"
				}
				if ok {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("line row %d %q, listing row %d, cursor %d,%d\n%s", p, g.Text(max(p, 0)), listing, r, c,
						smoke.Readable(smoke.LastLines(screen.Text(), 6)))
				}
				time.Sleep(50 * time.Millisecond)
			}
			if _, err := control.WriteString("\x03print -r -- END$((1+1))\r"); err != nil {
				t.Fatal(err)
			}
			if err := screen.Await("END2", widgetBudget); err != nil {
				t.Fatalf("the line after: %v", err)
			}
		})
	}
}

// A listing declined at its question, where the line is not gone back to, is
// followed by the whole prompt on a row of its own (#6209). Measured
// 2026-10-06 against zsh 5.9.2 with a two-row prompt, `unsetopt
// alwayslastprompt`, `LISTMAX=2` and three matches: `n` is followed by `\r\n`,
// the ground, and every row of the prompt, so the question stays on the screen
// above it; with the option on the cursor goes back up to the line. Here the
// declined question went back up to the line's last row either way.
func TestADeclinedListingDrawsTheWholePromptAgainBelow(t *testing.T) {
	const rc = `f() { compadd alpha1 alpha2 alpha3 }
zle -C cw complete-word f; bindkey '^T' cw
unsetopt alwayslastprompt; LISTMAX=2
`
	control, screen := widgetSession(t, rc)
	if _, err := control.WriteString("a\x14\x14"); err != nil {
		t.Fatal(err)
	}
	if err := screen.Await("possibilities", widgetBudget); err != nil {
		t.Fatalf("no question: %v\n%s", err, smoke.Readable(smoke.LastLines(screen.Text(), 4)))
	}
	if _, err := control.WriteString("nb"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(widgetBudget)
	for {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		p := lastRowStarting(g, widgetMark)
		r, c := g.Cursor()
		if p >= 2 && g.Text(p) == widgetMark+"alphab" && r == p && c == len(widgetMark+"alphab") &&
			g.Text(p-1) == "HWROW" && strings.Contains(g.Text(p-2), "possibilities") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("line row %d %q, cursor %d,%d\n%s", p, g.Text(max(p, 0)), r, c,
				smoke.Readable(smoke.LastLines(screen.Text(), 6)))
		}
		time.Sleep(50 * time.Millisecond)
	}
	if _, err := control.WriteString("\x03print -r -- END$((1+1))\r"); err != nil {
		t.Fatal(err)
	}
	if err := screen.Await("END2", widgetBudget); err != nil {
		t.Fatalf("the line after: %v", err)
	}
}
