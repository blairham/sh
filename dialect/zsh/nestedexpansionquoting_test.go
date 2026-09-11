// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// An expansion written inside another one is read in the quoting the outer was
// written in, which decides whether a bare `{` opens a level.
//
// zsh balances a bare brace inside an *unquoted* expansion and stops at the
// first `}` inside a double-quoted one — that split is `BareBraceNestsInExpansion`
// and it was already right for an expansion written on its own. The nested
// spelling reached it through a lexer built with no quoting at all, so the
// double-quoted half was read as if the word stood bare: the `{` of `%F{070\}`
// opened a level, the expansion ran past the `}` that closes it, and `}+` came
// back as literal text beside a value one brace too long.
//
// Measured on zsh 5.9.2, 2026-09-11, with the option-free spelling of each:
//
//	unset w; print -r -- "${${:-${w::=a{b}c}}+}"   is empty, $w is `a{b`
//	unset w; print -r --  ${${:-${w::=a{b}c}}+}    is empty, $w is `a{b}c`
//
// which is the whole of the claim: the same text, the same shell, and the
// quotes the only difference.
//
// This is what powerlevel10k's prompt is made of — `${${:-${_p9k__s::=%F{\}}…}+}`
// appears in it dozens of times — so a prompt that measured its own width
// through `${(%%)…}` was measuring text this had already corrupted (#2048).
func TestANestedExpansionIsReadInTheQuotingAroundIt(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a bare brace in double quotes closes nothing",
			src:  `unset w; print -r -- "[${${:-${w::=a{b}c}}+]" "[$w]"`,
			want: "[a{bc+] [a{b]\n",
		},
		{
			// The same text with one more `}` behind it: the outer closed a
			// brace earlier than the writer meant, so the last two
			// characters are ordinary text.
			name: "the brace that follows is text",
			src:  `unset w; print -r -- "[${${:-${w::=a{b}c}}+}]" "[$w]"`,
			want: "[a{bc+}] [a{b]\n",
		},
		{
			// And the other half of the split, so the pair is what pins it:
			// unquoted the brace *does* open a level, the inner runs to the
			// second `}`, and the outer's `+` then makes the whole thing
			// empty.
			name: "unquoted the same text balances the brace",
			src:  `unset w; print -r -- ${${:-${w::=a{b}c}}+} "[$w]"`,
			want: "[a{b}c]\n",
		},
		{
			// The shape p10k writes: the brace the value carries is escaped
			// and the one after it closes the expansion.
			name: "an escaped brace after a bare one",
			src:  `unset w; print -r -- "[${${:-${w::=%F{070\}}}+}]" "[$w]"`,
			want: "[] [%F{070}]\n",
		},
		{
			// Off on its own, which is what says the fix is about the
			// nesting and not about the brace rule underneath it.
			name: "an expansion written on its own is unchanged",
			src:  `unset w; print -r -- "[${w::=a{b}c}]" "[$w]"`,
			want: "[a{bc}] [a{b]\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := runZsh(t, dir, tc.src)
			if st != 0 {
				t.Fatalf("status = %d, out %q", st, out)
			}
			if out != tc.want {
				t.Errorf("%s\noutput = %q\n  want   %q", tc.src, out, tc.want)
			}
		})
	}
}
