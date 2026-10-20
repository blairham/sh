// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// Where a pattern's units are characters, a byte that begins no character
// stands for itself and is never the first byte of a character the subject
// holds. See interp.patternOpts.eqPatternHere.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc` (#5153).
func TestARawByteIsNotPartOfACharacterInAPattern(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a lead byte and a star", `[[ é == $'\xc3'* ]] || print no`, "no\n"},
		{"inside two stars", `[[ é == *$'\xc3'* ]] || print no`, "no\n"},
		{"a lead byte and a question mark", `[[ é == $'\xc3'? ]] || print no`, "no\n"},
		{"a trail byte", `[[ é == *$'\xa9'* ]] || print no`, "no\n"},
		{"a lead byte of three", `[[ € == $'\xe2'* ]] || print no`, "no\n"},
		{"a byte against the same byte", `x=$'\xc3a'; [[ $x == $'\xc3'* ]] && print yes`, "yes\n"},
		{"a character against a character", `[[ Stéphane == *é* ]] && print yes`, "yes\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, errs := runZshUTF8(t, c.src)
			if out != c.want || errs != "" {
				t.Errorf("%s\n got %q, %q\nwant %q", c.src, out, errs, c.want)
			}
		})
	}
}
