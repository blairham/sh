// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import "testing"

// A `$'…'` here-document delimiter ends the document at what it **means**,
// not at the escapes as written (#5062).
//
// The delimiter was compared as its raw bytes, so `CONSUME <<$'E\tOF'` never met
// its terminator and the document ran to the end of the file.
//
// Measured 2026-09-29 against the reference for this dialect, script files
// under `env -i PATH=/usr/bin:/bin` with a scratch HOME and standard input on
// the null device. **The whole escape table is read**: eleven forms were put in
// a delimiter and in a word in each of zsh 5.9.2, bash 5.3.20 and ksh93u+, and
// every shell ends the document at exactly what that same shell makes of the
// escapes in a word — including the forms the three disagree about. So the
// decoder is the shell's own and there is no smaller one that would do.
func TestAnAnsiCHeredocDelimiterEndsAtItsValue(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ name, src, want string }{
		{"a tab", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'E\\tOF'\nBODY\nE\tOF\necho DONE\n", "BODY\nDONE\n"},
		{"hex", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'EO\\x46'\nBODY\nEOF\necho DONE\n", "BODY\nDONE\n"},
		{"octal", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'EO\\106'\nBODY\nEOF\necho DONE\n", "BODY\nDONE\n"},
		{"a backslash", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'EO\\\\F'\nBODY\nEO\\F\necho DONE\n", "BODY\nDONE\n"},
		{"a quote", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'EO\\'F'\nBODY\nEO'F\necho DONE\n", "BODY\nDONE\n"},
		// The stripping operator reads the same delimiter.
		{"under <<-", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<-$'E\\tOF'\nBODY\nE\tOF\necho DONE\n", "BODY\nDONE\n"},
		// A delimiter part quoted and part not.
		{"part quoted", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<E$'\\tOF'\nBODY\nE\tOF\necho DONE\n", "BODY\nDONE\n"},
		// **Only the `$'…'` spans are decoded.** A single-quoted span beside
		// one keeps its backslash, so this delimiter is a literal `\t`
		// followed by `OF` — measured in all three references, which end the
		// document at the written text and not at the tab.
		{
			"a single-quoted span is not decoded",
			"while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<'E\\t'$'OF'\nBODY\nE\\tOF\necho DONE\n",
			"BODY\nDONE\n",
		},
		// **The other direction**: the text as written must not end it.
		{
			"the unexpanded text is body",
			"while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'E\\tOF'\nBODY\nE\\tOF\nE\tOF\necho DONE\n", "BODY\nE\\tOF\nDONE\n",
		},
		// The control that cannot fail either way, and is here to say so: a
		// delimiter with nothing to expand reads the same under both rules.
		{"nothing to expand", "while IFS= read -r l; do printf '%s\\n' \"$l\"; done <<$'EOF'\nBODY\nEOF\necho DONE\n", "BODY\nDONE\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, st := runKsh(t, dir, tc.src); out != tc.want || st != 0 {
				t.Errorf("out %q status %d, want %q at 0", out, st, tc.want)
			}
		})
	}
}
