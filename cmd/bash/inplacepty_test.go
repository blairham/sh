// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/terminfofixture"
)

// Terminfo's standard string order, for the descriptions written below.
const (
	capClear = 5
	capEl    = 6
	capCup   = 10
	capCub1  = 14
	capCuf1  = 17
	capDch1  = 21
	capSmir  = 31
	capRmir  = 42
	capDch   = 105
	capIch   = 108
	capCub   = 111
	capCuf   = 112
)

// described writes one description into a database the test owns and answers
// the session environment that names it.
func described(t *testing.T, name string, strs map[int]string) []string {
	t.Helper()
	top := 0
	for i := range strs {
		top = max(top, i)
	}
	db := terminfofixture.Database(t, terminfofixture.Description{Name: name, StrCount: top + 1, Strs: strs})
	return []string{"TERM=" + name, "TERMINFO=" + db}
}

// A change to the line is drawn in place, with the terminal's own insert and
// delete sequences, the way bash's editor draws it. Measured 2026-10-07
// through a pseudo-terminal, bash 5.3.20 `--norc -i` with this test's prompt,
// after `echo one` was run and `echo three` typed (#6332):
//
//	key            xterm                 wy50 (insert mode, dch1)      adm3a (no el)
//	Backspace      \b\e[K                \b\eT                         \b \b
//	Up             \b×4\e[1Pon\e[C       \b×4\eWon^L                   \b×4one \b
//	Down           \b×3\e[1@thr\e[C      \b×3\eq \er\bthr^L            \b×3thre
//	^B^B ^W        \b\b\e[2P             \b\b\eW\eW                    \b\bre  \b×4
//	^Y             \e[2@th               \eq  \er\b\bth                thre\b\b
//	^U             \r\e[C×4re\e[K\b\b    \r^L×4re\eT\b\b               \r^L×4re + 7 spaces + \r^L×4
//
// and under xterm `abc yy`, ^B^B, ^W is `\b×4\e[4P` where `abcd yy` is the
// tail written again: room is closed only while twice the suffix covers it.
//
// Here every one of them was the tail written again and `\e[0m\e[J`, with
// counted moves, on every terminal.
func TestAChangeIsDrawnInPlace(t *testing.T) {
	type key struct{ send, want string }
	for _, c := range []struct {
		name string
		strs map[int]string
		keys []key
	}{
		{"xterm-like", map[int]string{
			capClear: "\x1b[H\x1b[2J", capEl: "\x1b[K", capCup: "\x1b[%i%p1%d;%p2%dH",
			capCub1: "\b", capCuf1: "\x1b[C", capDch1: "\x1b[P", capSmir: "\x1b[4h", capRmir: "\x1b[4l",
			capDch: "\x1b[%p1%dP", capIch: "\x1b[%p1%d@", capCub: "\x1b[%p1%dD", capCuf: "\x1b[%p1%dC",
		}, []key{
			{"\x7f", "\b\x1b[K"},
			{"\x1b[A", "\b\b\b\b\x1b[1Pon\x1b[C"},
			{"\x1b[B", "\b\b\b\x1b[1@thr\x1b[C"},
			{"\x02\x02", "\b\b"},
			{"\x17", "\b\b\x1b[2P"},
			{"\x19", "\x1b[2@th"},
			{"\x15", "\r" + strings.Repeat("\x1b[C", 4) + "re\x1b[K\b\b"},
			// And the edge of the delete rule: four taken in front of two
			// is deleted, where five in front of two was written again.
			{"\x0b", "\x1b[K"},
			{"abc yy", "abc yy"},
			{"\x02\x02", "\b\b"},
			{"\x17", "\b\b\b\b\x1b[4P"},
		}},
		{"insert mode", map[int]string{
			capClear: "\x1b*", capEl: "\x1bT", capCup: "\x1b=%p1%' '%+%c%p2%' '%+%c",
			capCub1: "\b", capCuf1: "\f", capDch1: "\x1bW", capSmir: "\x1bq", capRmir: "\x1br",
		}, []key{
			{"\x7f", "\b\x1bT"},
			{"\x1b[A", "\b\b\b\b\x1bWon\f"},
			{"\x1b[B", "\b\b\b\x1bq \x1br\bthr\f"},
			{"\x02\x02", "\b\b"},
			{"\x17", "\b\b\x1bW\x1bW"},
			{"\x19", "\x1bq  \x1br\b\bth"},
			{"\x15", "\r\f\f\f\fre\x1bT\b\b"},
		}},
		{"no el", map[int]string{
			capClear: "\x1a", capCup: "\x1b=%p1%' '%+%c%p2%' '%+%c", capCub1: "\b", capCuf1: "\f",
		}, []key{
			{"\x7f", "\b \b"},
			{"\x1b[A", "\b\b\b\bone \b"},
			{"\x1b[B", "\b\b\bthre"},
			{"\x02\x02", "\b\b"},
			{"\x17", "\b\bre  \b\b\b\b"},
			{"\x19", "thre\b\b"},
			{"\x15", "\r\f\f\f\fre       \r\f\f\f\f"},
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			control, screen := interruptSessionWith(t, "", described(t, "inplaceterm", c.strs)...)
			t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
			press := func(k key) {
				t.Helper()
				at := len(screen.Text())
				if _, err := control.WriteString(k.send); err != nil {
					t.Fatal(err)
				}
				deadline := time.Now().Add(interruptBudget)
				for !strings.HasSuffix(screen.Text()[at:], k.want) {
					if time.Now().After(deadline) {
						t.Fatalf("after %q: drew %q, want it to end %q", k.send, screen.Text()[at:], k.want)
					}
					time.Sleep(5 * time.Millisecond)
				}
			}
			if _, err := control.WriteString("echo one\r"); err != nil {
				t.Fatal(err)
			}
			awaitInterruptPrompt(t, screen, "echo one")
			press(key{"echo three", "echo three"})
			for _, k := range c.keys {
				press(k)
			}
		})
	}
}
