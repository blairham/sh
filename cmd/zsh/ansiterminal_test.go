// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"testing"

	"github.com/blairham/sh/internal/terminfofixture"
)

// Terminfo's standard string order, for the descriptions these tests write.
const (
	capClear = 5
	capEl    = 6
	capEd    = 7
	capCud1  = 11
	capCub1  = 14
	capCuf1  = 17
	capCuu1  = 19
	capCud   = 107
	capCub   = 111
	capCuf   = 112
	capCuu   = 114
)

// movingTerminal is the environment of a session on a terminal that moves the
// cursor the ANSI way and has no attributes and no keys: xterm's movement and
// erase sequences, written into a database the test owns.
//
// A session whose test is about what the editor draws needs one. The line
// editor moves with the description's own sequences (#6325), so under
// `TERM=dumb`, which has none, it draws the way zsh does there — writing over
// the prompt, never moving up — and a test reading a screen grid of a
// multi-row prompt would be reading that instead of the feature it is about.
func movingTerminal(t *testing.T) []string {
	t.Helper()
	db := terminfofixture.Database(t, terminfofixture.Description{
		Name:     "movingterm",
		StrCount: capCuu + 1,
		Strs: map[int]string{
			capClear: "\x1b[H\x1b[2J", capEl: "\x1b[K", capEd: "\x1b[J",
			capCud1: "\n", capCub1: "\b", capCuf1: "\x1b[C", capCuu1: "\x1b[A",
			capCud: "\x1b[%p1%dB", capCub: "\x1b[%p1%dD",
			capCuf: "\x1b[%p1%dC", capCuu: "\x1b[%p1%dA",
		},
	})
	return []string{"TERM=movingterm", "TERMINFO=" + db}
}
