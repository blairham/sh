// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `${(F)a}` — join on newlines — as a script in this dialect writes it.
//
// The substrate carries the join and `interp/newlinejoinflag_test.go` grades
// it there. What is here is what only this dialect can answer: the array
// base a subscript is counted from, the escape set the `(p)` flag beside it
// reads, and the whole line as the issue wrote it, through `print` rather
// than through a `printf` format.
//
// Measured on zsh 5.9.2, 2026-09-25. #4452.
func TestTheNewlineJoinFlagInThisDialect(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// The issue's two lines, verbatim. Both refused with status 1
		// before the flag was carried.
		{"an array", `a=(x y z); print -r -- "${(F)a}"`, "x\ny\nz\n"},
		{"the positionals", `set -- a b c; print -r -- "${(F)@}"`, "a\nb\nc\n"},
		{"and the positionals by name", `set -- a b c; print -r -- "${(F)argv}"`, "a\nb\nc\n"},

		// A subscript selects before the join runs, and this dialect counts
		// from one — which is what makes these rows this package's rather
		// than the substrate's.
		{"one element", `a=(x y z); print -r -- "${(F)a[2]}"`, "y\n"},
		{"a range", `a=(x y z); print -r -- "${(F)a[1,2]}"`, "x\ny\n"},
		{"the whole array by subscript", `a=(x y z); print -r -- "${(F)a[@]}"`, "x\ny\nz\n"},

		// An association joins its values, the keys being a separate letter.
		{"an association's values", `typeset -A h=(k1 v1); print -r -- "${(F)h}"`, "v1\n"},

		// The separator this flag stands for is already a newline, so the
		// flag that reads a *written* argument's escapes has nothing to do
		// beside it. These rows would answer a backslash and an `n` if the
		// shorthand had been expanded into those two characters and re-read.
		{"the print flag in front of it", `a=(x y z); print -r -- "${(pF)a}"`, "x\ny\nz\n"},
		{"and behind it", `a=(x y z); print -r -- "${(Fp)a}"`, "x\ny\nz\n"},
		{"a j argument it does not own", `a=(x y z); print -r -- "${(pj:\n:F)a}"`, "x\ny\nz\n"},

		// And the round trip the flag exists for: `(F)` builds the text and
		// `(f)` takes it apart again.
		{"the inverse flag reads it back", `a=(x y z); v="${(F)a}"; printf "[%s]" "${(f)v}"`, "[x][y][z]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
