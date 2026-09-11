// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// This shell's answer to which arm of an alternation a longest prefix trim
// takes: the one written first, which is its alone in the panel.
//
// Measured 2026-09-11 on zsh 5.9.2, each row a `-c` of its own with `x=abc`.
// The neighbors are here because they are what say it is a search order
// rather than a second length rule — the first arm matching as much as it
// can, and a later arm taken where the rest of the pattern needs it.
func TestALongestPrefixTrimTakesTheArmWrittenFirst(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`x=abc; printf '[%s]' "${x##(a|ab)}"`, "[bc]"},
		{`x=abc; printf '[%s]' "${x##(ab|a)}"`, "[c]"},
		{`x=abc; printf '[%s]' "${x##(a|ab|abc)}"`, "[bc]"},
		{`x=abc; printf '[%s]' "${x##(ab|a|abc)}"`, "[c]"},
		{`x=abc; printf '[%s]' "${x##(a*|ab)}"`, "[]"},
		{`x=abc; printf '[%s]' "${x##(a|ab)c}"`, "[]"},
		{`x=abc; printf '[%s]' "${x##(|a)}"`, "[abc]"},
		{`x=abc; printf '[%s]' "${x##*(a|ab)}"`, "[bc]"},
		{`x=abc; printf '[%s]' "${x##(a|ab)(b|bc)}"`, "[c]"},
		{`x=abc; printf '[%s]' "${x##((a|ab))}"`, "[bc]"},
		{`x=abab; printf '[%s]' "${x##(ab|abab)}"`, "[ab]"},
		// The other three trims do not ask: the shortest prefix is the
		// shortest whichever arm came first, and both suffix trims take the
		// longest — measured, an empty first arm does not stop `%%`.
		{`x=abc; printf '[%s]' "${x#(a|ab)}"`, "[bc]"},
		{`x=abc; printf '[%s]' "${x#(ab|a)}"`, "[bc]"},
		{`x=abc; printf '[%s]' "${x%%(|bc)}"`, "[a]"},
		{`x=abc; printf '[%s]' "${x%(c|bc)}"`, "[ab]"},
		// A bar standing outside every group is an alternation here too,
		// and its arms are read in the same order — but only when it
		// arrived live, which `${~L}` is what does.
		{`L='a|ab'; x=abc; printf '[%s]' "${x##${~L}}"`, "[bc]"},
		{`L='ab|a'; x=abc; printf '[%s]' "${x##${~L}}"`, "[c]"},
		// The same match seen from the other side.
		{`x=abc; printf '[%s]' "${(M)x##(a|ab)}"`, "[a]"},
		{`setopt extendedglob; x=abc; : ${x##(#b)(a|ab)}; printf '[%s]' "$match[1]"`, "[a]"},
	} {
		out, _ := answersRun(t, tc.src)
		if got := strings.TrimSpace(out); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.src, got, tc.want)
		}
	}
}
