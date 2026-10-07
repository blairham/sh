// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"strings"
	"testing"
)

// A description's delay is NULs at the terminal's speed, n × speed ÷ 9000 of
// them for n milliseconds, and nothing without a speed. Measured 2026-10-07
// on zsh 5.9.2 at 9600 bits per second, `TERM=vt100`: 2 for `$<2>`, 3 for
// `$<3>`, 53 for `$<50>` (#6315).
func TestADelayIsWrittenAsNULsAtTheTerminalsSpeed(t *testing.T) {
	for _, c := range []struct {
		in    string
		speed int
		want  string
	}{
		{"\x1b[J$<50>", 9600, "\x1b[J" + strings.Repeat("\x00", 53)},
		{"\x1b[m$<2>", 9600, "\x1b[m\x00\x00"},
		{"\x1b[K$<3>", 9600, "\x1b[K\x00\x00\x00"},
		{"\x1b[J$<50>", 38400, "\x1b[J" + strings.Repeat("\x00", 213)},
		{"a$<5*/>b", 9600, "a\x00\x00\x00\x00\x00b"},
		{"\x1b[J$<50>", 0, "\x1b[J"},
		{"$<x>", 9600, "$<x>"},
	} {
		if got := padded(c.in, c.speed); got != c.want {
			t.Errorf("padded(%q, %d) = %q, want %q", c.in, c.speed, got, c.want)
		}
	}
}
