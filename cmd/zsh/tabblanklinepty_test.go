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

// Tab at an empty prompt types a tab, and the question about a long listing
// waits for its answer and then takes its own row off the screen (#6119).
//
// Measured 2026-10-05 through a pseudo-terminal against zsh 5.9.2, rendered
// cell by cell under a two-row prompt: Tab on an empty line with no
// completion system loaded leaves a tab in the line, drawn to column 8; with
// `LISTMAX=3` and `x` typed, Tab asks `do you wish to see all N
// possibilities`, waits, and a declining key erases the question and puts the
// cursor back on the line, while `y` lists from the question's row. Here the
// empty-line Tab completed every command on PATH and asked about them, and a
// declined question stayed on the screen with a fresh prompt under it.
func TestTabOnABlankLineAndTheListQuestion(t *testing.T) {
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
	type typing = interface{ WriteString(string) (int, error) }
	send := func(t *testing.T, c typing, keys string) {
		t.Helper()
		if _, err := c.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
	}

	t.Run("a tab on an empty line", func(t *testing.T) {
		control, screen := widgetSession(t)
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "\t")
		until(t, screen, "the tab drawn to its stop", func(g *cellgrid.Grid, p int) bool {
			r, c := g.Cursor()
			return r == p && c == 8
		})
		if strings.Contains(screen.Text(), "possibilities") {
			t.Errorf("a Tab on an empty line asked about a listing:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 4)))
		}
	})

	// The completion system's half: a function that sets `tab` in
	// `compstate[insert]` has the key typed rather than a word completed,
	// whatever it added — measured against zsh 5.9.2, `ab` and `^T` bound to
	// such a widget leave `ab^T`. It is how `insert-tab` works when the
	// completion system is loaded.
	t.Run("a completion that asks for the key", func(t *testing.T) {
		control, screen := widgetSession(t,
			`cf() { compstate[insert]=tab; compadd foo }; zle -C cw complete-word cf; bindkey '^T' cw`)
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "ab\x14")
		until(t, screen, "the key typed", func(g *cellgrid.Grid, p int) bool {
			r, c := g.Cursor()
			return g.Text(p) == mark+" ab^T" && r == p && c == 8
		})
	})

	for _, row := range []struct{ name, answer, echo string }{
		{"declined", "x", "x"},
		{"declined by a control key", "\x18", "n"},
	} {
		t.Run("the question "+row.name, func(t *testing.T) {
			control, screen := widgetSession(t, "LISTMAX=3")
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			send(t, control, "x\t")
			until(t, screen, "the question", func(g *cellgrid.Grid, p int) bool {
				return strings.HasPrefix(g.Text(p+1), "zsh: do you wish to see all ")
			})
			// It waits: nothing more is drawn until a key answers it.
			if !screen.Quiet(widgetMark, 750*time.Millisecond) {
				t.Fatalf("a prompt was drawn before the question was answered:\n%s",
					smoke.Readable(smoke.LastLines(screen.Text(), 4)))
			}
			send(t, control, row.answer)
			until(t, screen, "the question gone and the cursor back on the line", func(g *cellgrid.Grid, p int) bool {
				r, c := g.Cursor()
				return g.Text(p) == mark+" x" && strings.TrimSpace(g.Text(p+1)) == "" && r == p && c == 5
			})
			// Taken off the screen, with the line drawn again where it was
			// rather than under it: one prompt, no question.
			g := render(screen)
			prompts := 0
			for r := range g.Rows() {
				if strings.HasPrefix(g.Text(r), "zsh: do you wish") {
					t.Errorf("the question is still on the screen at row %d", r)
				}
				if strings.HasPrefix(g.Text(r), mark) {
					prompts++
				}
			}
			if prompts != 1 {
				t.Errorf("%d prompts on the screen, want the one the question was asked under", prompts)
			}
			// And the answer is echoed as zsh echoes it: the key, or `n` for
			// one that cannot be drawn.
			if !strings.Contains(screen.Text(), "lines)? "+row.echo+"\r") {
				t.Errorf("the answer was not echoed as %q:\n%q", row.echo, smoke.LastLines(screen.Text(), 3))
			}
		})
	}

	t.Run("the question answered yes", func(t *testing.T) {
		control, screen := widgetSession(t, "LISTMAX=3")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		send(t, control, "x\t")
		until(t, screen, "the question", func(g *cellgrid.Grid, p int) bool {
			return strings.HasPrefix(g.Text(p+1), "zsh: do you wish to see all ")
		})
		mark := len(screen.Text())
		send(t, control, "y")
		until(t, screen, "the listing", func(g *cellgrid.Grid, p int) bool { return len(screen.Text()) > mark+40 })
		g := render(screen)
		for r := range g.Rows() {
			if strings.HasPrefix(g.Text(r), "zsh: do you wish") {
				t.Fatalf("the question is still on the screen after yes, at row %d", r)
			}
		}
	})
}
