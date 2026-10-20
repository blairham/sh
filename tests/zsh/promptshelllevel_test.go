// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `%L` draws `$SHLVL`, and it draws the **parameter** rather than a count of
// its own — #4965.
//
// Measured 2026-09-27 on zsh 5.9.2, `-f`:
//
//	print -P '%L'; print $SHLVL          3    3
//	SHLVL=42; print -P '%L'              42
//	SHLVL=5;  print -P '|%2L|'           |5|
//	SHLVL=5;  print -P '%(4L.deep.…)'    deep
//
// The second row is the one that settles how it may be implemented: an
// assignment moves the escape, so this reads the name and does not count
// shells. That is also what makes it the same fact the `L` **test letter**
// already asked — the last row runs both spellings off one assignment, so a
// reading that drifted between them would part here rather than in a prompt.
//
// The count row is not decoration either. Every numeric code in this table
// takes its digits as an argument, and `%2L` is `5` and not `05` or `5` cut
// to two — so this code takes a count and ignores it, which is what
// `%?` and `%j` do and what a padding implementation would get wrong.
func TestThePromptShellLevelEscapeDrawsTheParameter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"the parameter's value", "SHLVL=3\nprint -P '%L'\n", "3\n"},
		{"an assignment moves it", "SHLVL=42\nprint -P '%L'\n", "42\n"},
		{"the expansion route draws it too", "SHLVL=7\nprint -rP -- \"[${(%):-%L}]\"\n", "[7]\n"},
		{"a count in front is taken and ignored", "SHLVL=5\nprint -P '|%2L|'\n", "|5|\n"},
		{"the test letter asks the same question", "SHLVL=5\nprint -P '%(4L.deep.shallow)'\n", "deep\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out, st := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("out = %q status = %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
