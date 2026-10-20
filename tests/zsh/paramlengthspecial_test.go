// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/internal/dialecttest"
)

// A `-` or `?` behind a `${#` is the name here and it stays the name when text
// is left over behind it, where the other five re-read the `#` as the
// parameter `$#`. Measured 2026-09-12 on zsh 5.9.2 from a script file under
// `env -i` with a scratch HOME and ZDOTDIR, `set -- p q r` so `$#` is 3:
//
//	$ zsh s.sh          # echo "${#-w}"
//	s.sh:2: bad substitution
//	$ zsh s.sh          # echo "${#-:-x}"
//	4
//
// bash 5.3, bash 3.2, bash-as-`sh`, dash, ksh93 and BusyBox ash all answer 3
// to both. The two readings agree by accident at two parameters, which is what
// the corpus row written that way could not see: `$-` is also two characters
// in bash and ksh93.
func TestALengthOverASpecialNameIsFinalHere(t *testing.T) {
	if !zsh.Dialect().ParamLengthOverASpecialNameIsFinal {
		t.Error("zsh keeps the length reading when text is left over behind the name")
	}
	for _, tc := range []struct {
		src, out string
		fails    bool
	}{
		// The stray word: the name is `-`, and `w` is not an operator.
		{src: `set -- p q r; echo "${#-w}"`, fails: true},
		{src: `set -- p q r; echo "${#?w}"`, fails: true},
		// A real operator behind the name applies to the name, not to `$#`.
		// `$?` is `0` here, so its length is 1 and the `:-` never fires.
		{src: `set -- p q r; echo "${#?:-x}"`, out: "1\n"},
		// Bare, the name is a length in every dialect — which is what says
		// the flag is about the leftover rather than about the name.
		{src: `set -- p q r; echo "${#?}"`, out: "1\n"},
		// The boundary: a special that is not an operator has no fallback
		// under either value, so this refuses here as it does everywhere.
		{src: `set -- p q r; echo "${#$w}"`, fails: true},
		{src: `set -- p q r; echo "${#!w}"`, fails: true},
	} {
		out, _, err := preset.Combined(t, dialecttest.Base{}, tc.src)
		switch {
		case tc.fails && out == "":
			t.Errorf("%q: no diagnostic, want a refusal", tc.src)
		case !tc.fails && err != nil:
			t.Errorf("%q: %v", tc.src, err)
		case !tc.fails && out != tc.out:
			t.Errorf("%q: out = %q, want %q", tc.src, out, tc.out)
		}
	}
}
