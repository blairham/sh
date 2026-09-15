// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"strings"
	"testing"
)

// Raw mode turns off the kernel's newline translation, so the loop that turned
// it off does it. A line ending in a bare newline moves the cursor down and
// not back, and the next line starts under the end of the last one.
func TestANewlineIsReturnedAsWell(t *testing.T) {
	for _, tc := range []struct{ name, in, want string }{
		{"a plain newline is returned as well", "done\n", "done\r\n"},
		{"one that already was is left alone", "done\r\n", "done\r\n"},
		{"several, mixed", "a\nb\r\nc\n", "a\r\nb\r\nc\r\n"},
		{"nothing to do", "no newline here", "no newline here"},
		{"a return on its own is left alone", "\rredraw", "\rredraw"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			w := &crlf{w: &out}
			n, err := w.Write([]byte(tc.in))
			if err != nil {
				t.Fatal(err)
			}
			// The count is the caller's bytes: a writer claiming more than it
			// was given is a short write to everything that checks.
			if n != len(tc.in) {
				t.Errorf("wrote %d, want %d", n, len(tc.in))
			}
			if got := out.String(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The return and the newline may arrive in separate writes, which is exactly
// how the editor draws: it writes what it has and the terminator after it.
// A translator that only looked within one buffer would double the return.
func TestAReturnCarriesAcrossWrites(t *testing.T) {
	var out strings.Builder
	w := &crlf{w: &out}
	for _, part := range []string{"redraw\r", "\n", "next\n"} {
		if _, err := w.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := out.String(), "redraw\r\nnext\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A session with no stream on a side has nothing to translate, and wrapping
// nil would turn a stream that is deliberately absent into one that panics.
func TestTranslatingLeavesAnAbsentStreamAbsent(t *testing.T) {
	if got := translating(nil); got != nil {
		t.Errorf("translating(nil) = %v, want nil", got)
	}
}

// And the carry is dropped the moment something else has had the terminal.
//
// The state is a claim about what the *screen* ends with, not about what this
// writer last handed over, and the two part company as soon as a command runs:
// its output goes straight to the terminal and the terminal echoes the keys
// itself. A return remembered across that window is a return that is no longer
// on the screen, and the newline trusting it lands bare (#2861).
func TestAForgottenReturnIsNotCarried(t *testing.T) {
	var out strings.Builder
	w := &crlf{w: &out}
	for _, part := range []string{"redraw\r", "\n"} {
		if _, err := w.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
		w.forget()
	}
	// The extra return is the deliberate half of the trade: at column 0 it
	// costs nothing, where a missing one costs the start of every row after.
	if got, want := out.String(), "redraw\r\r\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
