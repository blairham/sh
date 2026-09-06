// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"
)

// The dialect turns the grammar on, so the real lines that need it run here.
//
// The substrate's tests name the flag — `NestedParamExpansion` — and this one
// names the shell, which is the only place that is allowed. It is worth
// having for the reason the element-selection one is: a flag nothing turns on
// is a construct nobody can write, and nothing in `syntax` or `interp` can
// notice that.
func TestNestedExpansionsAreThisDialects(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The plugin loader's own idiom, whole rather than through a
			// variable: an exclusion whose result a default falls back over.
			// `${${0:#$ZSH_ARGZERO}:-…}` is how it finds the file being
			// sourced, and until the grammar landed it was a bad
			// substitution here.
			"a pattern exclusion under a default, as a loader writes it",
			`v=/abs/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[WASABSOLUTE]",
		},
		{
			"and the relative path survives its own exclusion",
			`v=rel/p; printf "[%s]" "${${v:#/*}:-WASABSOLUTE}"`,
			"[rel/p]",
		},
		{
			"an operator on the result of an expansion",
			`v=abc; printf "[%s]" "${${v}#a}"`,
			"[bc]",
		},
		{
			"a flag group on each of two levels",
			`v=abc; printf "[%s]" "${(U)${(L)v}}"`,
			"[ABC]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if out, _ := runZsh(t, t.TempDir(), tc.src); out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}
