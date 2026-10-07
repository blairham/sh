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

// vt52Terminal is the environment of a session on a terminal with movement
// sequences of its own and no counted ones: vt52's, written into a database
// the test owns rather than read from the machine's. The indices are
// terminfo's standard string order.
func vt52Terminal(t *testing.T) []string {
	t.Helper()
	const (
		clear = 5
		el    = 6
		ed    = 7
		cud1  = 11
		cub1  = 14
		cuf1  = 17
		cuu1  = 19
	)
	db := terminfofixture.Database(t, terminfofixture.Description{
		Name:     "vt52",
		StrCount: cuu1 + 1,
		Strs: map[int]string{
			clear: "\x1bH\x1bJ", el: "\x1bK", ed: "\x1bJ",
			cud1: "\x1bB", cub1: "\x1bD", cuf1: "\x1bC", cuu1: "\x1bA",
		},
	})
	return []string{"TERM=vt52", "TERMINFO=" + db}
}

// On a terminal whose description has movement sequences of its own the line
// is drawn with them. Measured 2026-10-07 through a pseudo-terminal, bash
// 5.3.20 under `TERM=vt52` with this test's prompt, the bytes each key wrote
// (#6324):
//
//	echo abcdef, ^B ^B ^B   \eD \eD \eD           (cub1)
//	X                       Xdef\eD\eD\eD
//	^A                      \r\eC\eC\eC\eC        (cuf1, past bj> )
//	#                       #echo abcXdef\r\eC\eC\eC\eC\eC
//	^E                      \eC ×12
//
// Every one of them was ANSI here — `\b`, `\e[12D`, `\e[12C` — which a vt52
// does not understand.
func TestATerminalsOwnMovementIsUsed(t *testing.T) {
	control, screen := interruptSessionWith(t, "", vt52Terminal(t)...)
	t.Cleanup(func() { _, _ = control.WriteString("\x01\x0b") })
	from := len(screen.Text())
	press := func(key, want string) {
		t.Helper()
		at := len(screen.Text())
		if _, err := control.WriteString(key); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(interruptBudget)
		for !strings.HasSuffix(screen.Text()[at:], want) {
			if time.Now().After(deadline) {
				t.Fatalf("after %q: drew %q, want it to end %q", key, screen.Text()[at:], want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	right := func(n int) string { return strings.Repeat("\x1bC", n) }
	press("echo abcdef", "echo abcdef")
	press("\x02", "\x1bD")
	press("\x02", "\x1bD")
	press("\x02", "\x1bD")
	press("X", "Xdef\x1bD\x1bD\x1bD")
	press("\x01", "\r"+right(len(interruptMark)))
	press("#", "#echo abcXdef\r"+right(len(interruptMark)+1))
	press("\x05", right(12))
	if drawn := screen.Text()[from:]; strings.Contains(drawn, "\x1b[") || strings.Contains(drawn, "\b") {
		t.Errorf("an ANSI sequence or a backspace was written to a vt52:\n%s", smoke.Readable(drawn))
	}
}
