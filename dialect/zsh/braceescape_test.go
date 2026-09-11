// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// A backslash before the `}` that would close a `${ }` escapes it here too,
// and this dialect is the one the row has to be asserted on (#1966).
//
// It reads a quoted replacement operand as the *enclosing* quoting — it is
// the only preset that answers Semantics.ReplacementOperandTakesTheEnclosingQuoting
// with `yes` — so the two readings are a live difference here and the escape
// has to hold under the one this dialect picks. The neighbouring `\q` row is
// that axis itself, and it stays put: the brace joined the escape set and did
// not replace it.
//
// Measured on zsh 5.9.2, 2026-09-10. This is what powerlevel10k builds its
// `${(e)}` pattern out of: `_p9k_must_init` writes a run of `${NAME-<sep>\}`
// entries and reads the result back, and a kept backslash made that text
// `closing brace expected` rather than a prompt (#1927).
func TestABackslashEscapesTheClosingBraceInThisDialect(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a word operand", `print -r -- "${u-A\}B}"`, "A}B\n"},
		{"a replacement operand", `v=x; print -r -- "${v/x/A\}B}"`, "A}B\n"},
		{"the global form", `v=x; print -r -- "${v//x/A\}B}"`, "A}B\n"},
		{"an element replacement", `a=(x); print -r -- "${(@)a:/x/A\}B}"`, "A}B\n"},
		{"a pattern operand takes the freed brace as a literal", `w='a}b'; print -r -- "${w/a\}b/Z}"`, "Z\n"},

		// The axis, unmoved: this dialect keeps a backslash that stands
		// before a character the enclosing reading does not escape, where
		// bash and ksh remove it.
		{"but a backslash before an ordinary character stays", `v=x; print -r -- "${v/x/A\qB}"`, "A\\qB\n"},
		{"and before an opening brace", `v=x; print -r -- "${v/x/A\{B}"`, "A\\{B\n"},

		// What `_p9k_must_init:23` reduces to: text built with the escape,
		// then read again.
		{
			"text built with the escape re-reads as an expansion",
			"setopt extendedglob\nA=1\nB=2\nf() {\n" +
				"  local MATCH IFS pat\n" +
				`  IFS=$'\1' pat="${(@)${(@o)parameters[(I)A|B]}:/(#m)*/\${${(q)MATCH}-$IFS\}}"` + "\n" +
				`  IFS=$'\2' print -r -- "[${(e)pat}]"` + "\n}\nf\n",
			"[1\x012]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
