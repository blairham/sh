// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
	"github.com/blairham/sh/internal/smoke"
)

// A paste the terminal marked is text in the line, newlines and all, and
// nothing in it runs until Return is pressed.
//
// Until #5865 this was false whenever anything bound began with ESC — and
// macOS's `/etc/zshrc` binds the arrows by `$terminfo[kcuu1]`, so on that
// machine it was every session. The lookup that reads the bindings gave up on
// the paste's opening marker at `\e[2`: it dropped those three bytes, `00~` and the paste
// behind it were typed a key at a time, and the first newline ran the first
// line. Measured at v0.0.25 with this paste: `> 00~echo one`, then `zsh:
// command not found: 00~echo`, then `> echo two01~` left in the line.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2, aarch64-apple-darwin25.4.0) with `PS1=$'upper row\n> '` and a
// paste of `echo one⏎echo two`: both lines drawn in reverse video under the
// prompt, the cursor after `two`, nothing run; Return runs both.
//
// Asserted on a terminal model and not on the text the shell wrote: the bug's
// screen holds `echo one` and `echo two` too, and only where they are, and
// where the cursor is, tells the two apart. What ran is read off the files the
// two lines create, which nothing on the screen can fake.
func TestABracketedPasteIsTextInTheLineUntilReturn(t *testing.T) {
	pasteIsTextInTheLineUntilReturn(t, "", "")
}

// The same paste through a paste widget of the kind zsh distributes, which is
// #5880: with `bracketed-paste-magic` installed the first paste of a session
// still ran its first line, while zsh 5.9.2 draws both lines and runs
// nothing.
//
// That function is not shipped here and is not read here, so the session
// loads testdata/functions/paste-key-by-key instead — written for this test
// from zsh's documented zle interface, and asking for what that function
// asks for: to be loaded by its first call, which it knows from
// `$zsh_eval_context`; `zle .bracketed-paste NAME`; `$UNDO_CHANGE_NO`,
// `split-undo` and `zle undo N`; `zle -U -`; a loop over `read-command` with
// `$KEYS` and `$REPLY`; `zle self-insert -w`; and `zle -K`. Each of those
// was missing or wrong here, and any one of them is enough to fail this: the
// load alone left the widget doing nothing, so the paste arrived as typed
// keys and its newline ran `: > ran1`.
//
// The paste goes into a line that already holds `true `, because the widget
// empties the line to build in and then undoes back to the number it noted:
// on an empty line an undo that lost what was typed before the widget looks
// exactly like one that kept it.
//
// Measured 2026-10-04 through a pseudo-terminal against /opt/homebrew/bin/zsh
// (zsh 5.9.2): with this file bound as the widget, the same paste leaves
// `: > ran1⏎: > ran2` in the line and the cursor at its end, nothing run —
// as it does with zsh's own `bracketed-paste-magic`.
func TestAPasteThroughAPasteWidgetIsTextInTheLineUntilReturn(t *testing.T) {
	functions, err := filepath.Abs(filepath.Join("testdata", "functions"))
	if err != nil {
		t.Fatal(err)
	}
	pasteIsTextInTheLineUntilReturn(t, "fpath=("+functions+" $fpath)\n"+
		"autoload -Uz paste-key-by-key\n"+
		"zle -N bracketed-paste paste-key-by-key\n", "true ")
}

// pasteIsTextInTheLineUntilReturn types typed into a session started with rc,
// pastes two lines after it, and asserts the paste is drawn as text in the
// line, runs nothing, and runs both lines when Return is pressed. typed must
// leave each pasted line still creating its file when it runs.
func pasteIsTextInTheLineUntilReturn(t *testing.T, rc, typed string) {
	t.Helper()
	// The binding `/etc/zshrc` makes on a Mac, written out so the session has
	// it on every platform: the harness's `TERM=dumb` has no `kcuu1` for the
	// system file to bind, and a Linux runner has no such file. Without one
	// the lookup never reads past the ESC and this passes against the bug,
	// which is how its first version did.
	control, screen, home := jobNoticeSessionRC(t, "bindkey '^[OA' up-line-or-search\n"+rc, "zsh", "-i")
	const (
		first  = ": > ran1"
		second = ": > ran2"
	)
	grid := func() *cellgrid.Grid {
		g := cellgrid.New(100)
		_, _ = g.Write([]byte(screen.Text()))
		return g
	}
	promptRow := func(g *cellgrid.Grid) int {
		row := -1
		for r := range g.Rows() {
			if strings.HasPrefix(g.Text(r), jobNoticeMark) {
				row = r
			}
		}
		return row
	}
	if typed != "" {
		// Typed and drawn before the paste is sent, so the paste reaches an
		// editor that is reading keys.
		if _, err := control.WriteString(typed); err != nil {
			t.Fatalf("typing %q: %v", typed, err)
		}
		deadline := time.Now().Add(jobNoticeBudget)
		for g := grid(); promptRow(g) < 0 || g.Text(promptRow(g)) != jobNoticeMark+strings.TrimSpace(typed); g = grid() {
			if time.Now().After(deadline) {
				t.Fatalf("%q was not drawn:\n%s", typed, g)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	if _, err := control.WriteString("\x1b[200~" + first + "\n" + second + "\x1b[201~"); err != nil {
		t.Fatalf("pasting: %v", err)
	}

	// The paste drawn: the prompt's own row holding what was typed and the
	// first line, the row under it the second, and the cursor at the end of
	// the second.
	drawn := func(g *cellgrid.Grid) bool {
		p := promptRow(g)
		row, col := g.Cursor()
		return p >= 0 && g.Text(p) == jobNoticeMark+typed+first && g.Text(p+1) == second &&
			row == p+1 && col == len(second)
	}
	deadline := time.Now().Add(jobNoticeBudget)
	for g := grid(); !drawn(g); g = grid() {
		if time.Now().After(deadline) {
			row, col := g.Cursor()
			t.Fatalf("the paste was not drawn as two lines in the buffer; cursor at row %d, column %d:\n%s\nraw: %q",
				row, col, g, screen.Text())
		}
		time.Sleep(10 * time.Millisecond)
	}
	// And drawn as a paste, which is how a person can tell it has not run:
	// in reverse video, the way both shells that bracket a paste mark one.
	g := grid()
	p := promptRow(g)
	for _, at := range [][2]int{{p, len(jobNoticeMark + typed)}, {p + 1, 0}, {p + 1, len(second) - 1}} {
		if c := g.Cell(at[0], at[1]); !c.Reverse {
			t.Errorf("the pasted %q at row %d, column %d is not marked as pasted:\n%s\nraw: %q",
				c.Text, at[0], at[1], g, screen.Text())
		}
	}

	// Nothing ran. A quiet period rather than one look: the bug ran the first
	// line within milliseconds of the paste, so a file that is coming is here
	// well inside it.
	ran := func(name string) bool {
		_, err := os.Stat(filepath.Join(home, name))
		return err == nil
	}
	for quiet := time.Now().Add(500 * time.Millisecond); time.Now().Before(quiet); time.Sleep(10 * time.Millisecond) {
		if ran("ran1") || ran("ran2") {
			t.Fatalf("part of the paste ran before Return was pressed:\n%s", smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
	}

	// Return runs the text, both lines of it — which is what makes the
	// silence above a measurement and not a session that runs nothing.
	if _, err := control.WriteString("\r"); err != nil {
		t.Fatalf("pressing Return: %v", err)
	}
	for deadline := time.Now().Add(jobNoticeBudget); !ran("ran1") || !ran("ran2"); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("Return did not run both pasted lines (ran1 %v, ran2 %v):\n%s",
				ran("ran1"), ran("ran2"), smoke.Readable(smoke.LastLines(screen.Text(), 8)))
		}
	}
}
