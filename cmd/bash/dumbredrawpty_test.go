// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/blairham/sh/internal/smoke"
)

// On a `dumb` terminal the line is drawn with backspaces, spaces and carriage
// returns and no escape sequence, which is what bash's editor does on a
// terminal that cannot move the cursor right. Measured 2026-10-07 through a
// pseudo-terminal, bash 5.3.20 under `TERM=dumb` with this test's prompt, the
// bytes each key wrote (#6314):
//
//	Left Left Left   \b \b \b
//	X                Xdef\b\b\b
//	^K               ' ' ×3, \b×3   (the line was echo abcXdef)
//	^A               \r\rbj>
//	^F               \rbj> e
//
// Here every one of them was an ANSI cursor sequence, which a dumb terminal
// prints as text.
func TestADumbTerminalIsDrawnWithoutCursorMovement(t *testing.T) {
	control, screen := interruptSession(t, "")
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
	press("echo abcdef", "echo abcdef")
	press("\x02", "\b")
	press("\x02", "\b")
	press("\x02", "\b")
	press("X", "Xdef\b\b\b")
	press("\x0b", "   \b\b\b")
	press("\x01", "\r\r"+interruptMark)
	press("\x06", "\r"+interruptMark+"e")
	if drawn := screen.Text()[from:]; strings.Contains(drawn, "\x1b") {
		t.Errorf("an escape sequence was written to a dumb terminal:\n%s", smoke.Readable(drawn))
	}
}
