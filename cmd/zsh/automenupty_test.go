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

// A Tab pressed again on a word the last one left ambiguous starts a menu
// completion, where the options say so (#6197).
//
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 with a
// scratch rc, `x a` typed in a directory holding `always` and `auto`, the
// screen rendered after each key:
//
//	options                       Tab 1             Tab 2        Tab 3     Tab 4
//	(defaults)                    \a and listing    \a always    auto      always
//	unsetopt automenu             \a and listing    \a listing   \a listing
//	setopt menucomplete           \a always, listing  auto
//	unsetopt autolist             \a                \a always    auto
//	unsetopt autolist automenu    \a                \a           \a
//
// and the same through a `zle -C` widget whose function adds the two words,
// whose `compstate[insert]` it may set to decide for itself. Here the second
// Tab drew the listing again and left the line alone, on every route.
//
// Under a two-row prompt, cell by cell, so that a menu drawn over the row the
// listing is on, or a listing drawn over the upper prompt row, cannot pass.
func TestARepeatedTabStartsAMenu(t *testing.T) {
	mark := strings.TrimSpace(widgetMark)
	render := func(screen *smoke.Screen) *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	type typing = interface{ WriteString(string) (int, error) }
	// press sends keys and waits until the last prompt row holds line and
	// the row under it holds below, and the cursor is at the end of the line.
	press := func(t *testing.T, c typing, screen *smoke.Screen, keys, line, below string) {
		t.Helper()
		if _, err := c.WriteString(keys); err != nil {
			t.Fatalf("typing %q: %v", keys, err)
		}
		deadline := time.Now().Add(widgetBudget)
		for {
			g := render(screen)
			p := lastRowStarting(g, mark)
			if p >= 0 && g.Text(p) == mark+" "+line && strings.TrimSpace(g.Text(p+1)) == below {
				r, col := g.Cursor()
				if r == p && col == len(mark)+1+len(line) {
					if p == 0 || g.Text(p-1) != "HWROW" {
						t.Fatalf("the upper prompt row is gone after %q:\n%s", keys, smoke.Readable(smoke.LastLines(screen.Text(), 6)))
					}
					return
				}
			}
			if time.Now().After(deadline) {
				var rows []string
				for r := range g.Rows() {
					rows = append(rows, g.Text(r))
				}
				t.Fatalf("after %q never saw line %q over %q; screen:\n%s", keys, line, below, strings.Join(rows, "\n"))
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	const dir = "mkdir am && : >am/always && : >am/auto && cd am"
	const listing = "always  auto"

	t.Run("by default, the Tab after the listing", func(t *testing.T) {
		control, screen := widgetSession(t, dir)
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t", "x a", listing)
		bells := strings.Count(screen.Text(), "\a")
		press(t, control, screen, "\t", "x always", listing)
		if got := strings.Count(screen.Text(), "\a") - bells; got != 1 {
			t.Errorf("the Tab that started the menu rang %d bells, want 1", got)
		}
		bells = strings.Count(screen.Text(), "\a")
		press(t, control, screen, "\t", "x auto", listing)
		press(t, control, screen, "\t", "x always", listing)
		if got := strings.Count(screen.Text(), "\a") - bells; got != 0 {
			t.Errorf("the Tabs that walked the menu rang %d bells, want none", got)
		}
		// And a key that is not a completion ends it: the next Tab is a
		// fresh completion of what is now on the line.
		press(t, control, screen, "\x05\x7f\x7f\x7f\x7f\x7f\t", "x a", listing)
	})

	t.Run("the control, with the option off", func(t *testing.T) {
		control, screen := widgetSession(t, dir, "unsetopt automenu")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t", "x a", listing)
		press(t, control, screen, "\tZ", "x aZ", listing)
	})

	t.Run("menucomplete starts it on the first Tab", func(t *testing.T) {
		control, screen := widgetSession(t, dir, "setopt menucomplete")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t", "x always", listing)
		press(t, control, screen, "\t", "x auto", listing)
	})

	t.Run("without autolist the second Tab starts it and nothing lists", func(t *testing.T) {
		control, screen := widgetSession(t, dir, "unsetopt autolist")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t", "x a", "")
		press(t, control, screen, "\t", "x always", "")
		press(t, control, screen, "\t", "x auto", "")
	})

	t.Run("without either nothing lists and nothing starts", func(t *testing.T) {
		control, screen := widgetSession(t, dir, "unsetopt autolist automenu")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t\t\tZ", "x aZ", "")
	})

	t.Run("reverse-menu-complete walks it back", func(t *testing.T) {
		control, screen := widgetSession(t, dir, "bindkey '^T' reverse-menu-complete")
		t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
		press(t, control, screen, "x a\t", "x a", listing)
		press(t, control, screen, "\t", "x always", listing)
		press(t, control, screen, "\x14", "x auto", listing)
		press(t, control, screen, "\x14", "x always", listing)
	})

	// Through a completion widget: the function runs for the Tab that lists
	// and the one that starts the menu, and its `compstate[insert]` is the
	// last word on how the matches go in.
	for _, row := range []struct {
		name, insert string
		lines        []string
	}{
		{"a widget that leaves compstate alone", "", []string{"x a", "x always", "x auto"}},
		{"a widget that asks for the prefix", "compstate[insert]=unambiguous", []string{"x a", "x a", "x a"}},
		{"a widget that asks for a menu at the second match", "compstate[insert]=menu:2", []string{"x auto", "x always", "x auto"}},
	} {
		t.Run(row.name, func(t *testing.T) {
			control, screen := widgetSession(t, dir,
				"w() { compadd always auto; "+row.insert+" }; zle -C w complete-word w; bindkey '^I' w")
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			press(t, control, screen, "x a\t", row.lines[0], listing)
			press(t, control, screen, "\t", row.lines[1], listing)
			press(t, control, screen, "\t", row.lines[2], listing)
		})
	}
}
