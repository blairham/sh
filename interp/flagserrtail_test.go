// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import "testing"

// What a flag-group error quotes: the rest of the word the group stands in,
// with its control characters made visible (#1647).
//
// The position was always right — it is the part a reader acts on — and the
// text was the expansion alone. Measured on zsh 5.9.2, 2026-09-12; the
// parser's half is TestAFlagGroupErrorCarriesTheRestOfItsWord, and this is
// what the report does with it.
func TestAFlagGroupErrorQuotesTheRestOfTheWord(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the closing quote of the region it stands in",
			`v=x; printf "[%s]" "[${(Z:x:)v}]"`,
			"testsh: error in flags near position 6 in '${(Z:x:)v}]\"'\n",
		},
		{
			"nothing where the expansion is the whole word",
			`v=x; printf "[%s]" ${(!)v}`,
			"testsh: error in flags near position 4 in '${(!)v}'\n",
		},
		{
			"and the word ends at the blank, not at the line",
			`v=x; printf "[%s]" ${(!)v} tail`,
			"testsh: error in flags near position 4 in '${(!)v}'\n",
		},
		{
			"a literal behind it comes with it",
			`v=x; printf "[%s]" ${(!)v}rest more`,
			"testsh: error in flags near position 4 in '${(!)v}rest'\n",
		},
		// A control character in that text is made visible rather than
		// written into the diagnostic — the same rendering `(V)` uses, so a
		// newline is `\n` and a backslash is left alone.
		{
			"a newline in the word is made visible",
			"v=x; printf \"[%s]\" \"a${(!)v}b\nsecond\"",
			"testsh: error in flags near position 4 in '${(!)v}b\\nsecond\"'\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs, st := flagsRun(t, tc.src)
			if errs != tc.want {
				t.Errorf("stderr = %q, want %q", errs, tc.want)
			}
			if out != "" || st == 0 {
				t.Errorf("out=%q st=%d, want the word abandoned at a nonzero status", out, st)
			}
		})
	}
}
