// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// A key whose edit has nothing to act on rings in one dialect and not the
// other (#6240). See EditorStyle.BellRingsWhenAnEditHasNothingToActOn for the
// measurement every row here is taken from.
//
// Each row is run twice, with and without the key, and the bells counted in
// both: what the key wrote is the difference, so a bell some other part of the
// read rang is not mistaken for this one.
func TestAKeyWithNothingToActOnRings(t *testing.T) {
	const (
		line  = "echo ab"
		start = line + "\x01"
	)
	for _, c := range []struct {
		name, setup, key string
		history          []string
		rings            bool
	}{
		{"^D at the end", line, "\x04", nil, true},
		{"Delete at the end", line, "\x1b[3~", nil, true},
		{"^F at the end", line, "\x06", nil, true},
		{"Right at the end", line, "\x1b[C", nil, true},
		{"^B at the start", start, "\x02", nil, true},
		{"Left at the start", start, "\x1b[D", nil, true},
		{"Backspace at the start", start, "\x7f", nil, true},
		{"^H at the start", start, "\x08", nil, true},
		{"^W at the start", start, "\x17", nil, true},
		{"^U at the start", start, "\x15", nil, true},
		{"^T at the start", start, "\x14", nil, true},
		{"^T on one character", "a", "\x14", nil, true},
		{"^Y with nothing killed", line, "\x19", nil, true},
		{"Down with no history", "", "\x1b[B", nil, true},
		{"^N on the newest", "", "\x0e", []string{"true"}, true},
		{"Up on the oldest", "\x1b[A", "\x1b[A", []string{"true"}, true},
		{"^P on the oldest", "\x10", "\x10", []string{"true"}, true},

		// The rows that do not ring, which are as much the measurement.
		{"Up with no history", "", "\x1b[A", nil, false},
		{"^P with no history", "", "\x10", nil, false},
		{"M-f at the end", line, "\x1bf", nil, false},
		{"^E at the end", line, "\x05", nil, false},
		{"^K at the end", line, "\x0b", nil, false},
		{"M-d at the end", line, "\x1bd", nil, false},
		{"M-b at the start", start, "\x1bb", nil, false},
		{"^A at the start", start, "\x01", nil, false},
		{"M-Delete at the start", start, "\x1b\x7f", nil, false},
		{"^T at the end", line, "\x14", nil, false},

		// And the same keys with something to act on.
		{"^D in the middle", line + "\x02", "\x04", nil, false},
		{"^F in the middle", start, "\x06", nil, false},
		{"Backspace at the end", line, "\x7f", nil, false},
		{"Up with history", "", "\x1b[A", []string{"true"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			bells := func(rings bool, keys string) int {
				var out strings.Builder
				e := &editor{in: typing(keys + "\r"), out: &out, ringsOnNothingToActOn: rings}
				for _, h := range c.history {
					e.remember(h)
				}
				if _, err := e.readLine(drawPrompt("$ ")); err != nil {
					t.Fatal(err)
				}
				return strings.Count(out.String(), bell)
			}
			before := bells(true, c.setup)
			got := bells(true, c.setup+c.key) - before
			want := 0
			if c.rings {
				want = 1
			}
			if got != want {
				t.Errorf("in the dialect that rings, the key rang %d times, want %d", got, want)
			}
			if got := bells(false, c.setup+c.key) - bells(false, c.setup); got != 0 {
				t.Errorf("in the dialect that does not, the key rang %d times, want 0", got)
			}
		})
	}
}

// vi insert mode rings for none of it: measured with `set -o vi` in bash
// 5.3.20, Backspace at the start and `^D` at the end are silent.
func TestAKeyWithNothingToActOnIsSilentInViInsertMode(t *testing.T) {
	for _, keys := range []string{"\x7f", "ab\x04", "\x15"} {
		var out strings.Builder
		e := &editor{
			in: typing(keys + "\r"), out: &out, ringsOnNothingToActOn: true,
			vi: func() bool { return true },
		}
		if _, err := e.readLine(drawPrompt("$ ")); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(out.String(), bell); n != 0 {
			t.Errorf("%q in vi insert mode rang %d times, want 0", keys, n)
		}
	}
}
