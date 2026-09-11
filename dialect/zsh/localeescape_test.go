// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// A `\u` escape naming a code point the locale cannot hold is **refused**:
// this shell reports `character not in range`, writes what came before the
// escape and nothing after it, and abandons the script with the status it
// already had.
//
// Measured 2026-09-11 against zsh 5.9.2, bytes read with `od`. Three details
// are each a way the answer could have been got wrong and are pinned
// separately: the complaint is located as the *shell* rather than as `echo`,
// which is the tell that this is a fact about reading a word; the status is
// **0**, so a script cannot see it in `$?`; and a subshell absorbs the
// abandonment the way it absorbs any other, so the enclosing script goes on
// (#1851).
//
// In a UTF-8 locale — where a person's terminal is, and where the panel
// agrees — the character is written, and this shell was already right there.
func TestAUnicodeEscapeOutsideTheLocaleIsRefused(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{
			"the complaint, then what came before it",
			`LC_ALL=C; echo 'a\u00e9Z'`,
			"zsh:1: character not in range\na\n",
		},
		{
			// Located as the shell rather than as `echo`, and the next line
			// never runs.
			"and the script is abandoned",
			`LC_ALL=C; echo 'a\u00e9Z'; echo AFTER`,
			"zsh:1: character not in range\na\n",
		},
		{
			// One complaint however many escapes the word holds, and the
			// text stops at the first: a refusal ends the text rather than
			// skipping a character.
			"two escapes draw one complaint",
			`LC_ALL=C; echo 'a\u00e9b\u00e9c'`,
			"zsh:1: character not in range\na\n",
		},
		{
			"nothing before it, so only the newline",
			`LC_ALL=C; echo '\u00e9Z'`,
			"zsh:1: character not in range\n\n",
		},
		{
			// A subshell absorbs it, so the enclosing script reaches AFTER.
			"a subshell absorbs the abandonment",
			`LC_ALL=C; ( echo 'a\u00e9Z' ); echo AFTER`,
			"zsh:1: character not in range\na\nAFTER\n",
		},
		{
			"a UTF-8 locale writes the character",
			`LC_ALL=en_US.UTF-8; echo 'a\u00e9Z'`,
			"a\u00e9Z\n",
		},
		{
			// ASCII is representable in every encoding, so the boundary is
			// the code point rather than the escape.
			"ASCII is in range even in C",
			`LC_ALL=C; echo 'a\u0041Z'`,
			"aAZ\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("%s = %q, want %q", tc.src, out, tc.want)
			}
		})
	}
}

// The status a refused escape leaves is **0**, which is the part a reader
// would guess wrong: it looks like an error and does not fail.
//
// Measured three ways, because "the status is 0" and "the status is whatever
// it already was" agree on the first probe and part company on the second: a
// failing command before the refusal leaves 3 behind, and the script still
// exits 0.
func TestARefusedEscapeLeavesStatusZero(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"after a success", `LC_ALL=C; true; echo 'a\u00e9Z'`},
		{"after a failure, so the 3 is not carried out", `LC_ALL=C; (exit 3); echo 'a\u00e9Z'`},
		{"and with a line after it that never runs", `LC_ALL=C; (exit 3); echo 'a\u00e9Z'; echo LATER`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, t.TempDir(), tc.src)
			if st != 0 {
				t.Errorf("status %d, want 0 — abandoned, not failed (output %q)", st, out)
			}
		})
	}
}
