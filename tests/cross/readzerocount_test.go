// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What `read -n 0` reads in each dialect with a count letter (#7036). bash 5.3
// reads nothing and leaves the line for the next reader; ksh93 takes nothing
// from the stream either; BusyBox ash reads the line, as though no count were
// given. See interp.Semantics.ReadCountOfZeroIsNoCount.
//
// Every row is a script run from -c on 2026-10-31 against bash 5.3.20,
// ksh93u+ 2012-08-01 and BusyBox ash 1.37.0 in the pinned alpine image, and
// the wanted output is what that shell printed. ksh93's rows print only the
// next read: what it assigns for a count of zero is bytes it never read,
// different on every run.
func TestACountOfZeroReadsWhatTheShellsRead(t *testing.T) {
	const next = `printf 'ab\ncd\n' | { v=x; read -n 0 v; read w; echo "st=$? [$v][$w]"; }`
	for _, tc := range []struct{ dialect, name, src, want string }{
		{"bash", "the next read", next, "st=0 [][ab]\n"},
		{"bash", "no newline", `printf ab | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=0 []\n"},
		{"bash", "nothing at all", `printf '' | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=0 []\n"},
		{
			"bash", "a deadline with the pipe held open",
			`{ printf ab; sleep 1; } | { v=x; read -t 0.2 -n 0 v; echo "st=$? [$v]"; }`,
			"st=0 []\n",
		},
		{"ksh", "the next read", `printf 'ab\ncd\n' | { read -n 0 v; read w; echo "[$w]"; }`, "[ab]\n"},
		{"ash", "the next read", next, "st=0 [ab][cd]\n"},
		{"ash", "no newline", `printf ab | { v=x; read -n 0 v; echo "st=$? [$v]"; }`, "st=1 [ab]\n"},
	} {
		t.Run(tc.dialect+"/"+tc.name, func(t *testing.T) {
			t.Parallel()
			out, st, err := presets[tc.dialect].Combined(t, dialecttest.Base{
				Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin"},
			}, tc.src)
			if err != nil || st != 0 || out != tc.want {
				t.Errorf("got %q (status %d, err %v), want %q", out, st, err, tc.want)
			}
		})
	}
}
