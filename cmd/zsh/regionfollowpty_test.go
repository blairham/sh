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

// `region_highlight`'s offsets follow the line when the editor edits it, and
// stay put when a widget assigns to it.
//
// Before #5874 nothing moved them, so a highlight set before an edit in front
// of it landed off the text it was set for. Measured 2026-10-04 against zsh
// 5.9.2 under `zsh -i` on a pseudo-terminal with this startup file, pressing
// each key and then ^U:
//
//	^Xa  RHA=[0 2 bold|3 5 underline|5 6 standout memo=m]   delete-char at 0
//	^Xb  RHB=[1 3 bold|3 3 underline|3 3 standout memo=m]   kill-line from 3
//	^Xc  RHC=[1 3 bold|4 6 underline|6 7 standout memo=m]   LBUFFER+=X at 0
//	^Xd  then Z, then ^Xs    RHS=[2 4 bold|5 7 underline]   a typed key
//
// The last row is the editor's own key, with no widget around it, and is
// the half a widget-only fix would miss. It is checked on the screen too,
// before any widget runs again: zsh draws the `bold` on `bc` of `Zabcdefgh`,
// where offsets left behind would put it on `ab`.
func TestRegionHighlightFollowsTheEditorsEdits(t *testing.T) {
	const rc = `rh() { print -r -- "$1=[${(j:|:)region_highlight}]" }
setup() { BUFFER=abcdefgh; CURSOR=$1; region_highlight=('1 3 bold' '4 6 underline' '6 7 standout memo=m') }
ta() { setup 0; zle .delete-char; rh RHA }
tb() { setup 3; zle .kill-line; rh RHB }
tc() { setup 0; LBUFFER+=X; rh RHC }
td() { BUFFER=abcdefgh; CURSOR=0; region_highlight=('1 3 bold' '4 6 underline') }
ts() { rh RHS }
for f in ta tb tc td ts; do zle -N $f; done
bindkey '^Xa' ta; bindkey '^Xb' tb; bindkey '^Xc' tc; bindkey '^Xd' td; bindkey '^Xs' ts
`
	// A session each, and each case's keys in one write: a widget that
	// prints hands the terminal back to its line discipline for the print,
	// and a key that arrives while it is taking the terminal back is a key
	// the kernel drops. Keys that arrive together are read together, before
	// any widget runs.
	for _, c := range []struct{ keys, want string }{
		{"\x18a", "RHA=[0 2 bold|3 5 underline|5 6 standout memo=m]"},
		{"\x18b", "RHB=[1 3 bold|3 3 underline|3 3 standout memo=m]"},
		{"\x18c", "RHC=[1 3 bold|4 6 underline|6 7 standout memo=m]"},
		{"\x18dZ\x18s", "RHS=[2 4 bold|5 7 underline]"},
	} {
		control, screen := widgetSession(t, rc)
		// ^U last, in the same write, so the session ends on an empty line
		// and the harness's `exit` is a command and not more of this one.
		if _, err := control.WriteString(c.keys + "\x15"); err != nil {
			t.Fatalf("typing %q: %v", c.keys, err)
		}
		// Every offset is the shell's, so the typed keys cannot satisfy this.
		if err := screen.Await(c.want, widgetBudget); err != nil {
			t.Fatalf("after %q the offsets are not\n%s\n%v\n%q", c.keys, c.want, err,
				smoke.LastLines(screen.Text(), 6))
		}
	}

	// The typed key's draw, read off a terminal model with no widget run
	// after it: the next widget would line the offsets up on its own.
	control, screen := widgetSession(t, rc)
	if _, err := control.WriteString("\x18dZ"); err != nil {
		t.Fatalf("typing: %v", err)
	}
	// Registered after the session's own, so it runs first: the line is
	// emptied before the harness types `exit`.
	t.Cleanup(func() { _, _ = control.WriteString("\x15") })
	boldOn := func(g *cellgrid.Grid) string {
		row := -1
		for r := range g.Rows() {
			if strings.HasPrefix(g.Text(r), widgetMark) {
				row = r
			}
		}
		var b strings.Builder
		for col := len(widgetMark); col < len(widgetMark)+9; col++ {
			if c := g.Cell(row, col); c.Bold {
				b.WriteString(c.Text)
			}
		}
		return b.String()
	}
	for deadline := time.Now().Add(widgetBudget); ; time.Sleep(10 * time.Millisecond) {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		if strings.Contains(g.String(), widgetMark+"Zabcdefgh") && boldOn(g) == "bc" {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("after a typed Z the bold is on %q, want \"bc\":\n%s", boldOn(g), g)
		}
	}
}
