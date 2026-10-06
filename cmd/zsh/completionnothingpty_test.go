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

// A completion widget whose function has no match leaves the line as it was,
// with the bell, and this editor's own completion is not asked after it
// (#6214).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, `x a` in a
// directory holding `always` and `auto` with Tab on a `zle -C` widget: a
// function that adds nothing, one whose words do not match, one that returns
// 1 and a widget whose function is not defined all write one bell and leave
// `x a` with nothing listed. Here each handed the key to the editor's own
// completion, which listed `always  auto` — and with `compinit` loaded, `cd `
// and Tab in a directory with no subdirectory became `cd a`.
//
// Under a two-row prompt, cell by cell. The Z typed after the Tab is what
// makes the wait discriminate: the line reads `x a` before the Tab as well.
func TestACompletionWidgetWithNoMatchLeavesTheLine(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	for _, row := range []struct{ name, rc string }{
		{"a function that adds nothing", "w() { : }; zle -C w complete-word w"},
		{"a function whose words do not match", "w() { compadd zzz yyy }; zle -C w complete-word w"},
		{"a function that fails", "w() { return 1; compadd always }; zle -C w complete-word w"},
		{"a widget whose function is not defined", "zle -C w complete-word nosuchfn"},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t,
				"mkdir am && : >am/always && : >am/auto && cd am", row.rc, "bindkey '^I' w")
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			bells := strings.Count(screen.Text(), "\a")
			if _, err := control.WriteString("x a\tZ"); err != nil {
				t.Fatalf("typing: %v", err)
			}
			deadline := time.Now().Add(widgetBudget)
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				p := lastRowStarting(g, mark)
				if p > 0 && g.Text(p) == mark+" x aZ" {
					if got := strings.TrimSpace(g.Text(p + 1)); got != "" {
						t.Errorf("drew %q under the line, want nothing listed", got)
					}
					if g.Text(p-1) != "HWROW" {
						t.Errorf("the upper prompt row is %q", g.Text(p-1))
					}
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("never saw the line left as typed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 6)))
				}
				time.Sleep(20 * time.Millisecond)
			}
			if got := strings.Count(screen.Text(), "\a") - bells; got != 1 {
				t.Errorf("rang %d bells, want 1", got)
			}
		})
	}
}
