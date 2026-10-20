// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// What brace expansion produced goes back into the word the parse cut here,
// rather than being read again as shell text — the other side of
// interp.Semantics.BraceOutputRereadAsText, which bash alone answers the other
// way.
//
// Measured 2026-09-22 against `/opt/homebrew/bin/zsh` 5.9.2, script files
// under `env -i PATH=/usr/bin:/bin LC_ALL=C`. ksh93u+ answers every row here
// identically, so this is not a zsh oddity: it is bash that re-reads.
//
// The two probes are the ones #4200 was filed from, and each fails a different
// way under the other reading. `$var{x,y}` would become the names `varx` and
// `vary`; `{Z..a}` would lose its backslash to quote removal, and
// `x{Z..a}y` would refuse the line outright on the backtick the range counted.
func TestBraceOutputGoesBackIntoTheWordRatherThanBeingReread(t *testing.T) {
	const set = `var=baz; varx=vx; vary=vy; `
	for _, tc := range []struct{ name, src, want string }{
		{"a bare name is still the name", set + `echo $var{x,y}`, "bazx bazy\n"},
		{"and so is a braced one", set + `echo ${var}{x,y}`, "bazx bazy\n"},
		{"a produced backslash is data", `printf '[%s]' {Z..a}`, "[Z][[][\\][]][^][_][`][a]"},
		{"and stands beside the word's own text", `printf '[%s]' x{Z..a}y`, "[xZy][x[y][x\\y][x]y][x^y][x_y][x`y][xay]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runZsh(t, t.TempDir(), tc.src); out != tc.want || st != 0 {
				t.Errorf("%s: got %q status %d, want %q", tc.src, out, st, tc.want)
			}
		})
	}
}
