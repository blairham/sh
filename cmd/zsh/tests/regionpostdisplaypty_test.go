// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package tests_test

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/cellgrid"
)

// A suggestion drawn after the line is not an edit to it, so it moves no
// `region_highlight` offset.
//
// #5930 made the offsets follow the editor's edits by diffing the line they
// were lined up with against the line as it is drawn — and what is drawn is
// the line *and* its `POSTDISPLAY`. So a suggestion appearing read as an
// insert at the cursor, and every offset at or past it moved by the
// suggestion's length: with zsh-autosuggestions and a syntax highlighter,
// typing `e` drew the whole of `echo $ZSH_VERSION` in the typed `e`'s color
// (#6412).
//
// Measured 2026-10-12 against zsh 5.9.2 under `zsh -i` in tmux 3.8, typing `e`
// with this startup file and reading `tmux capture-pane -p -e`:
//
//	"0 1 fg=red" "1 10 fg=8"   ESC[31m e ESC[90m cho hello ESC[39m
//	"1 10 fg=8"                e ESC[90m cho hello ESC[39m
//	"0 1 fg=red"               ESC[31m e ESC[39m cho hello
//
// With `PREDISPLAY="pre:"` set as well zsh draws `pre:` before the line and
// colors the line exactly as in the first row: an element without `P` counts
// from the buffer's start. One with `P` counts from the predisplay's, so
// `"P0 4 fg=blue" "P4 5 fg=red" "1 10 fg=8"` draws `pre:` blue, the `e` red
// and the suggestion gray (#6414, measured 2026-10-15 the same way). And with
// the widget bound to `e` alone, `e` and then `x` typed — the `x` the editor's
// own insert — the offsets follow that insert and not the suggestion:
// `ESC[31m ex ESC[90m cho hello ESC[39m`.
//
// Read here per cell off a terminal model, from where the text starts — the
// predisplay's cells included where there is one.
func TestASuggestionMovesNoRegionHighlight(t *testing.T) {
	const regions = `region_highlight=("0 1 fg=red" "1 10 fg=8")`
	widget := func(body string) string {
		return `_ins() { zle .self-insert; ` + body + `; }`
	}
	for _, c := range []struct {
		name, rc, keys, text string
		// want is each cell's foreground, as cellgrid names it.
		want []string
	}{
		{
			"red then gray",
			widget(`POSTDISPLAY="cho hello"; `+regions) + "\nzle -N self-insert _ins",
			"e", "echo hello",
			[]string{"1", "8", "8", "8", "8", "8", "8", "8", "8", "8"},
		},
		{
			"gray suggestion only",
			widget(`POSTDISPLAY="cho hello"; region_highlight=("1 10 fg=8")`) + "\nzle -N self-insert _ins",
			"e", "echo hello",
			[]string{"default", "8", "8", "8", "8", "8", "8", "8", "8", "8"},
		},
		{
			"red typed text only",
			widget(`POSTDISPLAY="cho hello"; region_highlight=("0 1 fg=red")`) + "\nzle -N self-insert _ins",
			"e", "echo hello",
			[]string{"1", "default", "default", "default", "default", "default", "default", "default", "default", "default"},
		},
		{
			"with a predisplay",
			widget(`PREDISPLAY="pre:"; POSTDISPLAY="cho hello"; `+regions) + "\nzle -N self-insert _ins",
			"e", "pre:echo hello",
			[]string{"default", "default", "default", "default", "1", "8", "8", "8", "8", "8", "8", "8", "8", "8"},
		},
		{
			"P elements count from the predisplay",
			widget(`PREDISPLAY="pre:"; POSTDISPLAY="cho hello"; region_highlight=("P0 4 fg=blue" "P4 5 fg=red" "1 10 fg=8")`) + "\nzle -N self-insert _ins",
			"e", "pre:echo hello",
			[]string{"4", "4", "4", "4", "1", "8", "8", "8", "8", "8", "8", "8", "8", "8"},
		},
		{
			"an editor insert after the suggestion",
			widget(`POSTDISPLAY="cho hello"; `+regions) + "\nzle -N my-e _ins\nbindkey e my-e",
			"ex", "excho hello",
			[]string{"1", "1", "8", "8", "8", "8", "8", "8", "8", "8", "8"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			control, screen := widgetSession(t, c.rc)
			// Registered after the session's own, so it runs first: the line
			// is emptied before the harness types `exit`.
			t.Cleanup(func() { _, _ = control.WriteString("\x15") })
			if _, err := control.WriteString(c.keys); err != nil {
				t.Fatalf("typing %q: %v", c.keys, err)
			}
			colors := func(g *cellgrid.Grid) []string {
				for r := range g.Rows() {
					text := g.Text(r)
					if !strings.HasPrefix(text, widgetMark) {
						continue
					}
					at := strings.Index(text, c.text)
					if at < 0 {
						return nil
					}
					var out []string
					for col := at; col < at+len(c.text); col++ {
						out = append(out, g.Cell(r, col).Fg.String())
					}
					return out
				}
				return nil
			}
			want := strings.Join(c.want, " ")
			for deadline := time.Now().Add(widgetBudget); ; time.Sleep(10 * time.Millisecond) {
				g := cellgrid.New(100)
				_, _ = g.Write([]byte(screen.Text()))
				got := strings.Join(colors(g), " ")
				if got == want {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("%q drawn in\n%s\nwant\n%s\n%s", c.text, got, want, g)
				}
			}
		})
	}
}
