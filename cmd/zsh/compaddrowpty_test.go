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

// A completion function's match that is not a file is listed whole, the typed
// directory included (#6200).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `x sub/a`
// typed with Tab on a widget running `compadd sub/aa sub/ab`: the listing is
// `sub/aa  sub/ab`. Here it was `aa  ab`, the editor taking the typed
// directory off the front of each row the way its own path completion does.
// Under a two-row prompt, cell by cell.
func TestACompaddMatchThatIsNotAFileIsListedWhole(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	control, screen := widgetSession(t, "w() { compadd sub/aa sub/ab }; zle -C w complete-word w; bindkey '^I' w")
	t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
	if _, err := control.WriteString("x sub/a\t"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	deadline := time.Now().Add(widgetBudget)
	for {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		p := lastRowStarting(g, mark)
		if p > 0 && g.Text(p) == mark+" x sub/a" && strings.TrimSpace(g.Text(p+1)) != "" {
			if got, want := strings.Join(strings.Fields(g.Text(p+1)), " "), "sub/aa sub/ab"; got != want {
				t.Errorf("listed %q, want %q", got, want)
			}
			if g.Text(p-1) != "HWROW" {
				t.Errorf("the upper prompt row is %q", g.Text(p-1))
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("never saw the listing:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 6)))
		}
		time.Sleep(20 * time.Millisecond)
	}
}
