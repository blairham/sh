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

// The four keys that ring in zsh because the edit fails, on a real terminal
// (#6247). Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2, a
// two-row prompt and `bindkey -e`: each writes `\a` and nothing else. Here
// each wrote nothing at all.
func TestAnEditThatFailsRingsTheBell(t *testing.T) {
	control, screen := widgetSession(t)
	mark := strings.TrimSpace(widgetMark)
	// Each row starts a fresh line with `^C`, which kills nothing, so `^Y`
	// still has nothing to yank. Two rows type the same line, which is why
	// the wait for the fresh prompt is a wait for one drawn after the `^C`.
	for _, row := range []struct{ name, setup, key string }{
		{"^T on one character", "a", "\x14"},
		{"^Y with nothing killed", "echo ab", "\x19"},
		{"Delete at the end", "echo ab", "\x1b[3~"},
		{"^D at the end, listing nothing", "echo ab", "\x04"},
	} {
		t.Run(row.name, func(t *testing.T) {
			deadline := time.Now().Add(widgetBudget)
			// The fresh line first, and only then the setup: a key sent
			// while the previous row's line is still on the screen can be
			// read as typed into it, and one sent before the editor is
			// reading again can be lost.
			fresh := len(screen.Text())
			if _, err := control.WriteString("\x03"); err != nil {
				t.Fatal(err)
			}
			for !strings.Contains(screen.Text()[fresh:], "HWROW") {
				if time.Now().After(deadline) {
					t.Fatalf("no fresh prompt after ^C:\n%s", smoke.Readable(screen.Text()[fresh:]))
				}
				time.Sleep(20 * time.Millisecond)
			}
			if _, err := control.WriteString(row.setup); err != nil {
				t.Fatal(err)
			}
			for {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				_, c := g.Cursor()
				if p := lastRowStarting(g, mark); p >= 1 && g.Text(p) == mark+" "+row.setup && c == len(widgetMark)+len(row.setup) {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("never saw the line typed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 4)))
				}
				time.Sleep(20 * time.Millisecond)
			}
			before := len(screen.Text())
			if _, err := control.WriteString(row.key); err != nil {
				t.Fatal(err)
			}
			for !strings.Contains(screen.Text()[before:], "\a") {
				if time.Now().After(deadline) {
					t.Fatalf("no bell; the key wrote:\n%s", smoke.Readable(screen.Text()[before:]))
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
	if _, err := control.WriteString("\x03"); err != nil {
		t.Fatal(err)
	}
}
