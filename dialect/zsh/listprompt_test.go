// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// What a paged listing's prompt draws, from where the listing stands (#6153).
// Measured 2026-10-06 through a pseudo-terminal against zsh 5.9.2 — see
// listprompt.go. The capitals pad to nine columns, nine and six, and a longer
// value is not cut.
func TestTheListPromptReportsWhereTheListingStands(t *testing.T) {
	r := bindkeyRunner(t, "zmodload zsh/complist\nLISTPROMPT='[%L][%M][%P] %l %m %p %%'\n")
	for _, c := range []struct {
		view repl.ListScrollView
		want string
	}{
		{
			repl.ListScrollView{LastLine: 39, Lines: 50, LastMatch: 989, Matches: 1000, Top: true},
			"[39/50    ][989/1000 ][Top   ] 39/50 989/1000 Top %",
		},
		{
			repl.ListScrollView{LastLine: 40, Lines: 50, LastMatch: 990, Matches: 1000},
			"[40/50    ][990/1000 ][80%   ] 40/50 990/1000 80% %",
		},
		{
			repl.ListScrollView{LastLine: 39, Lines: 706, LastMatch: 11335, Matches: 12000, Top: true},
			"[39/706   ][11335/12000][Top   ] 39/706 11335/12000 Top %",
		},
	} {
		got, ok := zsh.ListScrollPrompt(r, c.view)
		if !ok || got != c.want {
			t.Errorf("%+v drew %q, %v; want %q", c.view, got, ok, c.want)
		}
	}
	// Unset, or set without the module, it pages nothing.
	for _, src := range []string{"zmodload zsh/complist\n", "LISTPROMPT='x'\n"} {
		if _, ok := zsh.ListScrollPrompt(bindkeyRunner(t, src), repl.ListScrollView{}); ok {
			t.Errorf("%q pages a listing", src)
		}
	}
}
