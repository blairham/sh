// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A backslash before the `}` that would close a `${ }` escapes it here too,
// and this dialect is the other side of the axis it sits next to (#1966).
//
// A quoted replacement operand is read as a *word* here — this preset answers
// Semantics.ReplacementOperandTakesTheEnclosingQuoting with `no` — so the
// escape has to hold under that reading as well as under the enclosing one
// zsh picks. The `\\q` row is the axis itself and parts the two presets; the
// brace row does not, and that is the point: the brace joined the escape set
// the enclosing reading applies, so both readings answer it alike.
//
// Measured on bash 5.3.15 invoked as `bash`, 2026-09-10. bash 3.2 keeps the
// backslash and has no preset here to answer for it; the corpus row
// core/backslash-before-a-brace-in-a-quoted-operand records it.
func TestABackslashEscapesTheClosingBraceInThisDialect(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a word operand", `printf "[%s]" "${u-A\}B}"`, "[A}B]"},
		{"a replacement operand", `v=x; printf "[%s]" "${v/x/A\}B}"`, "[A}B]"},
		{"the global form", `v=x; printf "[%s]" "${v//x/A\}B}"`, "[A}B]"},
		{"a pattern operand takes the freed brace as a literal", `w='a}b'; printf "[%s]" "${w/a\}b/Z}"`, "[Z]"},

		// The axis, unmoved: a backslash before a character the enclosing
		// reading does not escape is removed here, where zsh keeps it.
		{"a backslash before an ordinary character is removed", `v=x; printf "[%s]" "${v/x/A\qB}"`, "[AqB]"},
		{"and before an opening brace", `v=x; printf "[%s]" "${v/x/A\{B}"`, "[A{B]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := answersRun(t, tc.src)
			if out != tc.want || st != 0 {
				t.Errorf("got %q (status %d), want %q at 0", out, st, tc.want)
			}
		})
	}
}
