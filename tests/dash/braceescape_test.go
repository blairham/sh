// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dash_test

import "testing"

// A backslash before the `}` that would close a `${ }` escapes it in the
// smallest shell too (#1966).
//
// There is no substitution operator here — dash refuses `${v/x/y}` — so this
// is only the word operand, which is the half dash has. It carries the row
// because it is the shell with nothing else in it: an escape that needed a
// dialect's machinery to work would not work here.
//
// Measured on dash, 2026-09-10.
func TestABackslashEscapesTheClosingBraceInThisDialect(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a word operand", `printf "[%s]" "${u-A\}B}"`, "[A}B]"},
		{"the colon form", `printf "[%s]" "${u:-A\}B}"`, "[A}B]"},
		{"an opening brace keeps its backslash", `printf "[%s]" "${u-A\{B}"`, `[A\{B]`},
		{"and so does an ordinary character", `printf "[%s]" "${u-A\qB}"`, `[A\qB]`},
		{"and an ordinary double-quoted run is untouched", `printf "[%s]" "A\}B"`, `[A\}B]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
