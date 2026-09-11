// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// A bare `{` inside an unquoted `${…}` opens nothing here: the expansion ends
// at the **first** `}` and whatever follows is more of the enclosing word.
//
// The substrate's tests name the flag — `BareBraceNestsInExpansion` — and
// this one names the shell. It is the half worth asserting on the *off* side:
// this shell gave zsh's answer here for as long as the scanner counted every
// brace, and nothing in `syntax` could notice, because the core and this
// preset agreed (#1587).
//
// Measured 2026-09-10 on bash 5.3.15 against the rest of the panel: this
// shell, bash 3.2.57 and dash answer one field, zsh 5.9.2 and ksh93 two.
func TestABareBraceDoesNotNestInAnUnquotedExpansion(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// One field: the operand stopped at the first `}` and `.z}`
			// arrived as literal text, so there is no group to expand.
			"the expansion ends at the first brace",
			`unset u; printf "[%s]" ${u:-{a,q}.z}`,
			"[{a,q.z}]",
		},
		{
			"a brace with nothing to expand ends it too",
			`unset u; printf "[%s]" ${u:-a{b}c}`,
			"[a{bc}]",
		},
		{
			// The set case says the same thing from the other side: the
			// value is the expansion, and `c}` is two more characters of
			// the word rather than the tail of an operand.
			"and the text after it stays in the word",
			`v=q; printf "[%s]" ${v:-a{b}c}`,
			"[qc}]",
		},
		{
			// Unbalanced is the discriminating shape: nesting leaves the
			// input running out, and this reading finishes the word.
			"an unbalanced brace is not an unterminated expansion",
			`unset u; printf "[%s]" ${u:-a{b}`,
			"[a{b]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runBash(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
