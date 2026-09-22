// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"testing"

	. "github.com/blairham/sh/interp"
)

// C's three punctuation escapes in a printf *format* (#4169).
//
// Measured 2026-09-22 under `LC_ALL=C` from a script file: bash 5.3.20 and
// bash 3.2.57 write the character alone, and zsh 5.9.2, dash 0.5.12 and
// BusyBox ash 1.37.0 write the backslash and the character. The three move
// together in every column, which is what keeps them one axis.
//
// The rows below move the axis both ways and check the two sites and the
// order, because the fall-through for an undefined escape reaches the same
// answer from the other side: with it set to drop the backslash, the
// character comes out whatever this says, and that is exactly why ksh93's
// column cannot measure this one.
func TestCsQuoteAndQuestionEscapesInAFormat(t *testing.T) {
	for _, tc := range []struct {
		name            string
		answer, unknown Answer
		src             string
		want            string
		refused         bool
	}{
		{
			name: "kept", answer: No, unknown: No,
			src: `printf '[\'"'"'][\"][\?]'`, want: `[\'][\"][\?]`,
		},
		{
			name: "read as C reads them", answer: Yes, unknown: No,
			src: `printf '[\'"'"'][\"][\?]'`, want: `[']["][?]`,
		},
		{
			// An escape the table does define is not this question, and must
			// not be reached through it.
			name: "a defined escape", answer: No, unknown: No,
			src: `printf 'a\tb'`, want: "a\tb",
		},
		{
			// Nor is anything else undefined: the axis claims three
			// characters and no more.
			name: "another undefined escape", answer: Yes, unknown: No,
			src: `printf '[\q]'`, want: `[\q]`,
		},
		{
			// The `%b` operand keeps the backslash however this is
			// answered, which is what makes it the format's question alone.
			name: "a %b operand", answer: Yes, unknown: No,
			src: `printf '%b' '[\?]'`, want: `[\?]`,
		},
		{
			// The order, and the whole of why one column is unmeasurable:
			// a `No` here falls through to the axis that drops a backslash
			// from every undefined escape, so the character comes out and
			// this one was never the reason.
			name: "no, and the fall-through drops it", answer: No, unknown: Yes,
			src: `printf '[\?]'`, want: `[?]`,
		},
		{
			// Which also says the axis is asked in front of that one rather
			// than behind it: unanswered has to refuse even where the
			// fall-through would have produced the same text.
			name: "unanswered", answer: Unspecified, unknown: Yes,
			src: `printf '[\?]'`, want: `[?]`, refused: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := printfSem()
			sem.PrintfQuoteAndQuestionEscapes = tc.answer
			sem.PrintfUnknownEscapeDropsTheBackslash = tc.unknown
			out, st := run(t, tc.src, func(r *Runner) { r.Semantics = &sem })
			if tc.refused {
				want := "sh: `printf` reading C's `\\'`, `\\\"` and `\\?` in a format: " +
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
