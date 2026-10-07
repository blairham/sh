// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
	"github.com/blairham/sh/internal/terminfofixture"
)

// vt52Terminal is vt52's description, written into a database the test owns:
// single steps and no counted moves, and tabs every eight columns.
func vt52Terminal(t *testing.T) []string {
	t.Helper()
	const ht = 134 // and `it`, the second number
	db := terminfofixture.Database(t, terminfofixture.Description{
		Name:     "vt52",
		Nums:     []int{terminfofixture.Absent, 8},
		StrCount: ht + 1,
		Strs: map[int]string{
			capClear: "\x1bH\x1bJ", capEl: "\x1bK", capEd: "\x1bJ",
			capCud1: "\x1bB", capCub1: "\x1bD", capCuf1: "\x1bC", capCuu1: "\x1bA",
			ht: "\t",
		},
	})
	return []string{"TERM=vt52", "TERMINFO=" + db}
}

// The line editor moves with the terminal's own sequences, chosen the way zsh
// chooses them. Measured 2026-10-07 through a pseudo-terminal, zsh 5.9.2 `-f
// -i` with this test's two-row prompt, typing `echo abcdef` a key at a time
// and then the keys below (#6325):
//
//	                 xterm-like       vt52                 dumb
//	c after e        \bec             \eDec                c
//	^B               \b               \eD                  \b
//	X                Xdef\b\b\b       Xdef\eD\eD\eD        Xdef\b\b\b
//	Backspace        \bdef \b ×4      \eDdef \eD ×4        \bdef \b ×4
//	^K               ' '×3 \b×3       ' '×3 \eD ×3         ' '×3 \b×3
//	^A               \e[8D            \r\eC\eC\eC\eC       \r + 4 spaces
//	^F               \e[1C            e                    e
//	^E               \e[7C            \t abc               cho abc
//	^U               \e[8D, ' '×8,    \r\eC×4, ' '×8,      \r, ' '×12,
//	                 \e[8D            \r\eC×4              \r + 4 spaces
//
// Every column was the first one here, with `\e[0m\e[J` for each erase and no
// redraw of the first key.
func TestTheEditorMovesTheWayTheTerminalAndZleDo(t *testing.T) {
	type key struct{ send, want string }
	common := func(back, right, home, toEnd, kill string) []key {
		return []key{
			{"e", "e"},
			{"c", ""},
			{"h", "h"},
			{"o", "o"},
			{" ", " "},
			{"a", "a"},
			{"b", "b"},
			{"c", "c"},
			{"d", "d"},
			{"e", "e"},
			{"f", "f"},
			{"\x02", back},
			{"\x02", back},
			{"\x02", back},
			{"X", "Xdef" + strings.Repeat(back, 3)},
			{"\x7f", back + "def " + strings.Repeat(back, 4)},
			{"\x0b", "   " + strings.Repeat(back, 3)},
			{"\x01", home},
			{"\x06", right},
			{"\x05", toEnd},
			{"\x15", kill},
		}
	}
	vt52Home := "\r" + strings.Repeat("\x1bC", 4)
	for _, c := range []struct {
		name  string
		term  func(*testing.T) []string
		first string
		keys  []key
	}{
		{
			"xterm-like", movingTerminal, "\bec",
			common("\b", "\x1b[1C", "\x1b[8D", "\x1b[7C", "\x1b[8D"+strings.Repeat(" ", 8)+"\x1b[8D"),
		},
		{
			"vt52", vt52Terminal, "\x1bDec",
			common("\x1bD", "e", vt52Home, "\t abc", vt52Home+strings.Repeat(" ", 8)+vt52Home),
		},
		{
			"dumb", func(*testing.T) []string { return []string{"TERM=dumb"} }, "c",
			common("\b", "e", "\r    ", "cho abc", "\r"+strings.Repeat(" ", 12)+"\r    "),
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			control, screen, _ := jobNoticeSessionOn(t, c.term(t), "", "zsh", "-i")
			t.Cleanup(func() { _, _ = control.WriteString("\x15") })
			for i, k := range c.keys {
				want := k.want
				if i == 1 {
					want = c.first
				}
				at := len(screen.Text())
				if _, err := control.WriteString(k.send); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(jobNoticeBudget)
				for screen.Text()[at:] != want {
					if time.Now().After(deadline) {
						t.Fatalf("after %q: drew %q, want %q\n%s", k.send, screen.Text()[at:], want,
							smoke.Readable(smoke.LastLines(screen.Text(), 3)))
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
		})
	}
}
