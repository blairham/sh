// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// The edits that ring in zsh because they fail, and the far larger set that
// rings in bash and not in zsh (#6247). See EditorStyle.BellRingsWhenAnEditFails
// for the measurement every row here is taken from.
//
// Each row is run with and without the key and the bells counted in both, as
// TestAKeyWithNothingToActOnRings does, so a bell some other part of the read
// rang is not mistaken for this one.
func TestAnEditThatFailsRingsInZsh(t *testing.T) {
	const (
		line  = "echo ab"
		start = line + "\x01"
	)
	style := EditorStyle{BellRingsWhenAnEditFails: true, ListOnControlD: true, TransposeAtTheStartSwapsTheFirstTwo: true}
	for _, c := range []struct {
		name, setup, key string
		rings            bool
	}{
		{"^T on one character", "a", "\x14", true},
		{"^Y with nothing killed", line, "\x19", true},
		{"Delete at the end", line, "\x1b[3~", true},
		{"^D at the end, listing nothing", line, "\x04", true},

		// The keys bash rings for and zsh does not.
		{"^F at the end", line, "\x06", false},
		{"Right at the end", line, "\x1b[C", false},
		{"^B at the start", start, "\x02", false},
		{"Backspace at the start", start, "\x7f", false},
		{"^W at the start", start, "\x17", false},
		{"^U at the start", start, "\x15", false},
		{"Down with no history", "", "\x1b[B", false},

		// And the same keys with something to act on.
		{"^T at the start swaps", start, "\x14", false},
		{"^Y with something killed", line + "\x17", "\x19", false},
		{"Delete in the middle", line + "\x02", "\x1b[3~", false},
		{"^D in the middle", line + "\x02", "\x04", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			bells := func(style EditorStyle, keys string) int {
				var out strings.Builder
				e := Shell{Editor: style}.newEditor(t.Context(), nil)
				e.in, e.out = typing(keys+"\r"), &out
				if _, err := e.readLine(drawPrompt("$ ")); err != nil {
					t.Fatal(err)
				}
				return strings.Count(out.String(), bell)
			}
			got := bells(style, c.setup+c.key) - bells(style, c.setup)
			want := 0
			if c.rings {
				want = 1
			}
			if got != want {
				t.Errorf("the key rang %d times, want %d", got, want)
			}
			// Without the field only the listing rings.
			plain := EditorStyle{ListOnControlD: true, TransposeAtTheStartSwapsTheFirstTwo: true}
			got = bells(plain, c.setup+c.key) - bells(plain, c.setup)
			want = 0
			if c.rings && c.key == "\x04" {
				want = 1
			}
			if got != want {
				t.Errorf("without the field the key rang %d times, want %d", got, want)
			}
		})
	}
}

// vi insert mode is not this: the keys are emacs's.
func TestAnEditThatFailsIsSilentInViInsertMode(t *testing.T) {
	for _, keys := range []string{"a\x14", "ab\x19"} {
		var out strings.Builder
		e := Shell{Editor: EditorStyle{BellRingsWhenAnEditFails: true}}.newEditor(t.Context(), nil)
		e.in, e.out = typing(keys+"\r"), &out
		e.vi = func() bool { return true }
		if _, err := e.readLine(drawPrompt("$ ")); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(out.String(), bell); n != 0 {
			t.Errorf("%q in vi insert mode rang %d times, want 0", keys, n)
		}
	}
}
