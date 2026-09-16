// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// BusyBox ash says something different about a `--word` than about a letter,
// which is the trap in this row — #2298.
//
// Measured 2026-09-16 on BusyBox ash 1.37.0 in the pinned Alpine image, with
// standard input on /dev/null:
//
//	ash -q          ash: illegal option -q
//	ash --badopt    ash: bad option '--badopt'
//
// Both at status 2, and a different verb with quotes around the word. A
// refusal written from the letter's sentence one spelling over would have
// been wrong here in two places at once, and this is the column this project
// has had wrong five separate times for want of being asked (#3228).
//
// The word is echoed whole, measured on `--xyz`, `--a` and `--login=x` as
// well.
func TestABadLongOptionIsQuotedAndIsNotTheLettersSentence(t *testing.T) {
	for _, tc := range []struct{ word, want string }{
		{"--badopt", "ash: bad option '--badopt'\n"},
		{"--xyz", "ash: bad option '--xyz'\n"},
		{"--a", "ash: bad option '--a'\n"},
		{"--login=x", "ash: bad option '--login=x'\n"},
		// The letter, for the contrast that makes the row a measurement.
		{"-q", "ash: illegal option -q\n"},
	} {
		t.Run(tc.word, func(t *testing.T) {
			var o, e bytes.Buffer
			sh := shell()
			sh.SystemStartupDirectory = t.TempDir()
			sh.Stdout, sh.Stderr = &o, &e
			code := driver.MainArgs(sh, []string{"ash", tc.word})
			if code != 2 {
				t.Errorf("status %d, want 2", code)
			}
			if o.Len() > 0 {
				t.Errorf("stdout = %q, want nothing", o.String())
			}
			if e.String() != tc.want {
				t.Errorf("stderr = %q, want %q", e.String(), tc.want)
			}
		})
	}
}
