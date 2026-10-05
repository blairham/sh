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
					ok = ok && listing == p-1
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
