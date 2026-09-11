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

// The third fault in the same neighborhood, and the one the second uncovered.
//
// A `${ }` written inside a double-quoted run brings its own quoting with it,
// so the `"` in `"${:-"Z"}"` is that expansion's rather than the run's closing
// one. The scan that looks for the run's end read it as the close, which left
// the nested `${` swallowed and uncounted while its `}` landed outside and
// closed the *enclosing* expansion. What followed fell out as literal text.
//
// This is what powerlevel10k's directory segment is written as —
// `${P9K_CONTENT::="%{d%}${:-"%B%F{039}"}…%{d%}"}` — and the prompt drew the
// `"}`, `}+}` and `}}}+}` tails of the expansions it closed early, with the
// leading path components gone (#2092). It could not be seen until #2074 made
// the marker replacement run at all, so it is a defect rather than a
// regression.
//
// Measured on zsh 5.9.2, 2026-09-11, under `env -i`. The fix is the core's —
// all six panel columns answer `[X{039}z]` to
// `printf '[%s]' "${u:-"${w:-"X{039}"}z"}"` — and these rows are here because
// this is the shell whose prompt is made of the shape.
func TestABracedExpansionInAQuotedRunKeepsItsOwnQuotes(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The assignment form the theme writes, in miniature: the value
			// is a quoted run holding a quoted nested expansion.
			name: "an assignment operand holding a quoted nested expansion",
			src:  `unset v; print -r -- "[${v::="a${:-"X{1}"}b"}]" "[$v]"`,
			want: "[aX{1}b] [aX{1}b]\n",
		},
		{
			// And with nothing unbalanced in the nested expansion at all,
			// so the row is about the quote rather than about the brace.
			name: "the same with nothing unbalanced inside",
			src:  `unset v; print -r -- "[${v::="A${:-"B"}C"}]" "[$v]"`,
			want: "[ABC] [ABC]\n",
		},
		{
			// The silent route, and the one the prompt takes: `${(%%)…}`
			// re-reads the text as a raw body, where there is no enclosing
			// word for the stray quote to unbalance. This wrote
			// `aX{1"}b"}` and exited 0.
			name: "through the prompt flag, which said nothing",
			src:  `setopt promptsubst; unset v; s='${v::="a${:-"X{1}"}b"}'; print -r -- "[${(%%)s}]" "[$v]"`,
			want: "[aX{1}b] [aX{1}b]\n",
		},
		{
			// Three levels, which is what the segment really nests to.
			name: "three levels of it",
			src:  `unset u w x; print -r -- "[${u:-"${w:-"${x:-"p{q}r"}s"}t"}]"`,
			want: "[p{q}rst]\n",
		},
		{
			// The neighbor the change must not move: a bare `{` in a
			// double-quoted expansion still opens no level, which is
			// TestANestedExpansionIsReadInTheQuotingAroundIt's subject and
			// the reading the step-over has to preserve.
			name: "a bare brace in the run still closes nothing",
			src:  `unset w; print -r -- "[${${:-${w::=a{b}c}}+]" "[$w]"`,
			want: "[a{bc+] [a{b]\n",
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
