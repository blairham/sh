// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// An escape a printf *format* does not define (#2904).
//
// The axis is moved both ways and left unanswered on one snippet, because a
// backslash that is always written and one that is always dropped look alike
// from a single row.
func TestAnUndefinedEscapeInAFormat(t *testing.T) {
	for _, tc := range []struct {
		name    string
		answer  Answer
		src     string
		want    string
		refused bool
	}{
		{
			name: "kept", answer: No,
			src: `printf '[\q][\z][\8][\-]'`, want: `[\q][\z][\8][\-]`,
		},
		{
			name: "dropped", answer: Yes,
			src: `printf '[\q][\z][\8][\-]'`, want: `[q][z][8][-]`,
		},
		{
			// A format ending in a backslash is not this question: there is
			// no character for it to have been in front of, and every
			// column writes it. So the axis must not be reached.
			name: "a trailing backslash, kept", answer: No,
			src: `printf 'a\'`, want: `a\`,
		},
		{
			name: "a trailing backslash, dropped", answer: Yes,
			src: `printf 'a\'`, want: `a\`,
		},
		{
			// Nor is an escape the table *does* define, at either site.
			name: "a defined escape", answer: Yes,
			src: `printf 'a\tb'`, want: "a\tb",
		},
		{
			// The `%b` operand's table is its own and keeps the backslash
			// however this is answered, which is what makes this the
			// format's question alone.
			name: "a %b operand, dropped", answer: Yes,
			src: `printf '%b' '[\q]'`, want: `[\q]`,
		},
		{
			name: "unanswered", answer: Unspecified,
			src: `printf '[\q]'`, want: `[\q]`, refused: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfUnknownEscapeDropsTheBackslash = tc.answer
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused {
				want := "sh: `printf` writing an undefined escape's character without its backslash: " +
					"the shells disagree here and no dialect was chosen\n" + tc.want
				if out != want || st != 2 {
					t.Errorf("got %q status %d, want %q and 2", out, st, want)
				}
				return
			}
			if out != tc.want || st != 0 {
				t.Errorf("got %q status %d, want %q and 0", out, st, tc.want)
			}
		})
	}
}
