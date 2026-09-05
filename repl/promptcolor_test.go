// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import "testing"

// What a color code writes, measured against zsh 5.9.2 through a pty with one
// code per prompt and the bytes compared as hex.
//
// The background half is the same arithmetic ten higher, and the two ranges
// above the first eight are their own runs of parameters rather than a
// modifier: 8 is not 0 made brighter, it is 90.
func TestWhatAColorCodeWrites(t *testing.T) {
	for _, tc := range []struct {
		layer PromptColor
		arg   string
		want  string
	}{
		{Foreground, "red", "\x1b[31m"},
		{Foreground, "black", "\x1b[30m"},
		{Foreground, "green", "\x1b[32m"},
		{Foreground, "yellow", "\x1b[33m"},
		{Foreground, "blue", "\x1b[34m"},
		{Foreground, "magenta", "\x1b[35m"},
		{Foreground, "cyan", "\x1b[36m"},
		{Foreground, "white", "\x1b[37m"},
		// A number says the same thing as the name.
		{Foreground, "2", "\x1b[32m"},
		{Foreground, "07", "\x1b[37m"},
		// The bright eight.
		{Foreground, "8", "\x1b[90m"},
		{Foreground, "15", "\x1b[97m"},
		// And everything above them, which the terminal takes as an index
		// rather than as a color of its own.
		{Foreground, "16", "\x1b[38;5;16m"},
		{Foreground, "200", "\x1b[38;5;200m"},
		{Foreground, "255", "\x1b[38;5;255m"},
		// Nothing in the braces is black, which is measured and is not what it
		// looks like: zsh drew `%F` and `%F{}` alike as `\e[30m`.
		{Foreground, "", "\x1b[30m"},
		// Anything the names and the range both refuse is the default. The
		// names are lower case and only lower case.
		{Foreground, "bogus", "\x1b[39m"},
		{Foreground, "Red", "\x1b[39m"},
		{Foreground, "256", "\x1b[39m"},
		{Foreground, "-1", "\x1b[39m"},
		{Background, "blue", "\x1b[44m"},
		{Background, "red", "\x1b[41m"},
		{Background, "5", "\x1b[45m"},
		{Background, "8", "\x1b[100m"},
		{Background, "200", "\x1b[48;5;200m"},
		{Background, "bogus", "\x1b[49m"},
		{Background, "", "\x1b[40m"},
	} {
		t.Run(tc.arg, func(t *testing.T) {
			if got := colorSequence(tc.layer, tc.arg); got != tc.want {
				t.Errorf("colorSequence(%v, %q) = %q, want %q", tc.layer, tc.arg, got, tc.want)
			}
		})
	}
}

// Where the code ends, which is what decides whether the text after it is text.
func TestWhereAColorCodesArgumentEnds(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		want     string
		wantEnd  int
	}{
		// The braces and everything in them belong to the code.
		{"braced", "{red}x", "red", 4},
		{"empty braces", "{}x", "", 1},
		// No braces at all: the argument is empty and the code ended before
		// the letters, which is measured — zsh drew `%Fred` as a color and
		// then `red`.
		{"unbraced", "red", "", -1},
		{"nothing after it", "", "", -1},
		// An opening brace with no closing one is the rest of the prompt,
		// because there is no later text for the rest to be text of.
		{"unterminated", "{red", "red", 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			arg, end := colorArgument([]rune(tc.in), 0)
			if arg != tc.want || end != tc.wantEnd {
				t.Errorf("colorArgument(%q) = %q, %d, want %q, %d", tc.in, arg, end, tc.want, tc.wantEnd)
			}
		})
	}
}
